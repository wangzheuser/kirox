package http

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

const pythonHTTPSProxyBridgeScript = `
import base64, secrets, select, socket, ssl, sys, threading, urllib.parse

upstream = sys.stdin.readline().strip()
u = urllib.parse.urlparse(upstream)
proxy_host = u.hostname
proxy_port = u.port or 443
upstream_user = urllib.parse.unquote(u.username or "")
upstream_pass = urllib.parse.unquote(u.password or "")
local_user = secrets.token_hex(12)
local_pass = secrets.token_hex(12)

def recv_until(sock):
    data = b""
    while b"\r\n\r\n" not in data and len(data) < 65536:
        chunk = sock.recv(4096)
        if not chunk:
            break
        data += chunk
    return data

def parse_headers(raw):
    lines = raw.decode("latin1", "replace").split("\r\n")
    headers = {}
    for line in lines[1:]:
        if not line or ":" not in line:
            continue
        k, v = line.split(":", 1)
        headers[k.strip().lower()] = v.strip()
    return lines[0], headers

def authorized(headers):
    value = headers.get("proxy-authorization", "")
    if not value.lower().startswith("basic "):
        return False
    try:
        raw = base64.b64decode(value.split(None, 1)[1]).decode()
    except Exception:
        return False
    return raw == (local_user + ":" + local_pass)

def upstream_connect(target):
    raw = socket.create_connection((proxy_host, proxy_port), timeout=12)
    raw.settimeout(12)
    conn = ssl.create_default_context().wrap_socket(raw, server_hostname=proxy_host)
    token = base64.b64encode((upstream_user + ":" + upstream_pass).encode()).decode()
    req = (
        "CONNECT " + target + " HTTP/1.1\r\n"
        "Host: " + target + "\r\n"
        "Proxy-Authorization: Basic " + token + "\r\n"
        "User-Agent: kirox-proxy-bridge\r\n"
        "\r\n"
    ).encode()
    conn.sendall(req)
    resp = recv_until(conn)
    status = resp.split(b"\r\n", 1)[0]
    if b" 200 " not in status:
        conn.close()
        raise RuntimeError(status.decode("latin1", "replace"))
    conn.settimeout(None)
    return conn

def pipe(left, right):
    try:
        while True:
            ready, _, _ = select.select([left, right], [], [], 60)
            if not ready:
                return
            for src in ready:
                dst = right if src is left else left
                data = src.recv(16384)
                if not data:
                    return
                dst.sendall(data)
    except Exception:
        pass
    finally:
        for item in (left, right):
            try:
                item.close()
            except Exception:
                pass

def target_for_plain(first_line, headers):
    parts = first_line.split()
    if len(parts) < 2:
        raise RuntimeError("bad request")
    parsed = urllib.parse.urlparse(parts[1])
    host = parsed.netloc or headers.get("host", "")
    if not host:
        raise RuntimeError("missing host")
    if ":" not in host:
        host += ":443" if parsed.scheme == "https" else ":80"
    path = urllib.parse.urlunparse(("", "", parsed.path or "/", parsed.params, parsed.query, ""))
    return host, path

def handle(client):
    try:
        client.settimeout(12)
        raw_req = recv_until(client)
        if not raw_req:
            return
        first_line, headers = parse_headers(raw_req)
        if not authorized(headers):
            client.sendall(b"HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
            return
        parts = first_line.split()
        if len(parts) < 3:
            client.sendall(b"HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
            return
        method = parts[0].upper()
        if method == "CONNECT":
            upstream = upstream_connect(parts[1])
            client.sendall(b"HTTP/1.1 200 Connection Established\r\n\r\n")
            client.settimeout(None)
            pipe(client, upstream)
            return

        target, path = target_for_plain(first_line, headers)
        upstream = upstream_connect(target)
        lines = raw_req.decode("latin1", "replace").split("\r\n")
        out = [method + " " + path + " HTTP/1.1"]
        for line in lines[1:]:
            if not line:
                break
            lower = line.split(":", 1)[0].strip().lower()
            if lower in ("proxy-authorization", "proxy-connection"):
                continue
            out.append(line)
        upstream.sendall(("\r\n".join(out) + "\r\n\r\n").encode("latin1"))
        client.settimeout(None)
        pipe(client, upstream)
    except Exception:
        try:
            client.sendall(b"HTTP/1.1 504 Gateway Timeout\r\nContent-Length: 0\r\n\r\n")
        except Exception:
            pass
    finally:
        try:
            client.close()
        except Exception:
            pass

listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
listener.bind(("127.0.0.1", 0))
listener.listen(50)
port = listener.getsockname()[1]
print("http://" + local_user + ":" + local_pass + "@127.0.0.1:" + str(port), flush=True)
while True:
    conn, _ = listener.accept()
    threading.Thread(target=handle, args=(conn,), daemon=True).start()
`

// newPythonHTTPSProxyBridge 启动基于系统 Python/OpenSSL TLS 栈的代理桥。
func newPythonHTTPSProxyBridge(raw string, timeout time.Duration) (*httpsProxyBridge, error) {
	python, err := lookPython()
	if err != nil {
		return nil, err
	}
	upstream, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("代理地址解析失败: %w", err)
	}
	cmd := exec.Command(python, "-u", "-c", pythonHTTPSProxyBridgeScript)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(stdin, raw+"\n"); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	_ = stdin.Close()

	type startResult struct {
		localURL string
		err      error
	}
	started := make(chan startResult, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		started <- startResult{localURL: strings.TrimSpace(line), err: err}
	}()
	select {
	case result := <-started:
		if result.err != nil || result.localURL == "" {
			_ = cmd.Process.Kill()
			return nil, fmt.Errorf("读取 Python 代理桥地址失败: %w", result.err)
		}
		bridge := &httpsProxyBridge{
			upstream: upstream,
			cmd:      cmd,
			localURL: result.localURL,
			timeout:  timeout,
		}
		bridge.markUsed()
		go func() {
			_ = cmd.Wait()
			bridge.closed.Store(true)
		}()
		return bridge, nil
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("启动 Python 代理桥超时")
	}
}

// lookPython 查找可用的系统 Python 解释器。
func lookPython() (string, error) {
	if path, err := exec.LookPath("python3"); err == nil {
		return path, nil
	}
	return exec.LookPath("python")
}
