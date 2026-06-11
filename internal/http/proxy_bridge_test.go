package http

import (
	"bufio"
	"encoding/base64"
	"io"
	"net"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestHTTPSProxyBridgeConnect 验证本地 HTTP 桥能通过上游 HTTPS 代理建立 CONNECT 隧道。
func TestHTTPSProxyBridgeConnect(t *testing.T) {
	disablePythonBridgeForTest(t)

	var gotAuth string
	var mu sync.Mutex
	upstream := httptest.NewTLSServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Proxy-Authorization")
		mu.Unlock()
		if r.Method != stdhttp.MethodConnect {
			t.Errorf("上游代理请求方法不符合预期: %s", r.Method)
		}
		if r.Host != "target.example.test:443" {
			t.Errorf("上游 CONNECT 目标不符合预期: %s", r.Host)
		}
		conn, rw := hijackProxyConn(t, w)
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		buf := make([]byte, 4)
		if _, err := io.ReadFull(rw, buf); err != nil {
			t.Errorf("读取隧道数据失败: %v", err)
			return
		}
		if string(buf) != "ping" {
			t.Errorf("隧道请求数据不符合预期: %q", string(buf))
		}
		_, _ = rw.WriteString("pong")
		_ = rw.Flush()
	}))
	defer upstream.Close()

	bridge := newTestHTTPSProxyBridge(t, upstream.URL)
	defer bridge.Close()

	conn := dialLocalBridge(t, bridge.localURL)
	defer conn.Close()
	if _, err := io.WriteString(conn, "CONNECT target.example.test:443 HTTP/1.1\r\nHost: target.example.test:443\r\n"+localProxyAuthHeader(t, bridge.localURL)+"\r\n"); err != nil {
		t.Fatalf("写入本地 CONNECT 失败: %v", err)
	}
	resp, err := stdhttp.ReadResponse(bufio.NewReader(conn), &stdhttp.Request{Method: stdhttp.MethodConnect})
	if err != nil {
		t.Fatalf("读取本地 CONNECT 响应失败: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusOK {
		t.Fatalf("本地 CONNECT 响应不符合预期: %s", resp.Status)
	}
	if _, err := io.WriteString(conn, "ping"); err != nil {
		t.Fatalf("写入隧道数据失败: %v", err)
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(conn, reply); err != nil {
		t.Fatalf("读取隧道响应失败: %v", err)
	}
	if string(reply) != "pong" {
		t.Fatalf("隧道响应不符合预期: %q", string(reply))
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("up-user:up-pass"))
	mu.Lock()
	defer mu.Unlock()
	if gotAuth != wantAuth {
		t.Fatalf("上游代理认证不符合预期: got %q, want %q", gotAuth, wantAuth)
	}
}

// TestHTTPSProxyBridgeRejectsMissingLocalAuth 验证本地桥不会暴露无认证开放代理。
func TestHTTPSProxyBridgeRejectsMissingLocalAuth(t *testing.T) {
	disablePythonBridgeForTest(t)

	upstream := httptest.NewTLSServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		t.Error("缺少本地认证时不应访问上游代理")
		w.WriteHeader(stdhttp.StatusOK)
	}))
	defer upstream.Close()

	bridge := newTestHTTPSProxyBridge(t, upstream.URL)
	defer bridge.Close()

	conn := dialLocalBridge(t, bridge.localURL)
	defer conn.Close()
	if _, err := io.WriteString(conn, "CONNECT target.example.test:443 HTTP/1.1\r\nHost: target.example.test:443\r\n\r\n"); err != nil {
		t.Fatalf("写入本地 CONNECT 失败: %v", err)
	}
	resp, err := stdhttp.ReadResponse(bufio.NewReader(conn), &stdhttp.Request{Method: stdhttp.MethodConnect})
	if err != nil {
		t.Fatalf("读取本地 CONNECT 响应失败: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusProxyAuthRequired {
		t.Fatalf("缺少本地认证应返回 407: %s", resp.Status)
	}
}

// TestBridgeHTTPSProxyURLReturnsLocalHTTPProxy 验证 HTTPS 上游会被转换成本地 HTTP 代理 URL。
func TestBridgeHTTPSProxyURLReturnsLocalHTTPProxy(t *testing.T) {
	disablePythonBridgeForTest(t)

	upstream := httptest.NewTLSServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusBadGateway)
	}))
	defer upstream.Close()

	raw := upstreamProxyURL(t, upstream.URL)
	localURL, err := bridgeHTTPSProxyURL(raw, time.Second)
	if err != nil {
		t.Fatalf("创建代理桥失败: %v", err)
	}
	parsed, err := url.Parse(localURL)
	if err != nil {
		t.Fatalf("本地代理 URL 解析失败: %v", err)
	}
	if parsed.Scheme != "http" || !strings.HasPrefix(parsed.Host, "127.0.0.1:") || parsed.User == nil {
		t.Fatalf("本地代理 URL 不符合预期: %s", localURL)
	}
}

// disablePythonBridgeForTest 让单元测试稳定覆盖纯 Go 桥实现。
func disablePythonBridgeForTest(t *testing.T) {
	t.Helper()
	old := enablePythonHTTPSProxyBridge
	enablePythonHTTPSProxyBridge = false
	t.Cleanup(func() {
		enablePythonHTTPSProxyBridge = old
	})
}

// newTestHTTPSProxyBridge 创建连接到 httptest 上游的代理桥。
func newTestHTTPSProxyBridge(t *testing.T, upstreamURL string) *httpsProxyBridge {
	t.Helper()
	bridge, err := newHTTPSProxyBridge(upstreamProxyURL(t, upstreamURL), time.Second)
	if err != nil {
		t.Fatalf("创建测试代理桥失败: %v", err)
	}
	bridge.insecure = true
	return bridge
}

// upstreamProxyURL 返回带上游认证的 HTTPS 代理 URL。
func upstreamProxyURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("上游代理 URL 解析失败: %v", err)
	}
	u.User = url.UserPassword("up-user", "up-pass")
	return u.String()
}

// dialLocalBridge 连接本地代理桥。
func dialLocalBridge(t *testing.T, raw string) net.Conn {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("本地代理 URL 解析失败: %v", err)
	}
	conn, err := net.DialTimeout("tcp", u.Host, time.Second)
	if err != nil {
		t.Fatalf("连接本地代理桥失败: %v", err)
	}
	return conn
}

// localProxyAuthHeader 返回本地代理桥认证头。
func localProxyAuthHeader(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("本地代理 URL 解析失败: %v", err)
	}
	password, _ := u.User.Password()
	token := base64.StdEncoding.EncodeToString([]byte(u.User.Username() + ":" + password))
	return "Proxy-Authorization: Basic " + token + "\r\n"
}

// hijackProxyConn 接管 httptest 代理连接。
func hijackProxyConn(t *testing.T, w stdhttp.ResponseWriter) (net.Conn, *bufio.ReadWriter) {
	t.Helper()
	hijacker, ok := w.(stdhttp.Hijacker)
	if !ok {
		t.Fatal("测试服务不支持 Hijacker")
	}
	conn, rw, err := hijacker.Hijack()
	if err != nil {
		t.Fatalf("Hijack 失败: %v", err)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	return conn, rw
}
