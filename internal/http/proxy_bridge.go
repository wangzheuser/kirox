package http

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	stdhttp "net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	utls "github.com/bogdanfinn/utls"
)

const (
	httpsProxyBridgeIdleTTL       = 2 * time.Minute
	httpsProxyBridgeReapInterval  = 30 * time.Second
	defaultHTTPSProxyBridgeDialTO = 60 * time.Second
)

var (
	httpsProxyBridgeMu    sync.Mutex
	httpsProxyBridgeCache = map[string]*httpsProxyBridge{}

	enablePythonHTTPSProxyBridge = true
)

type httpsProxyBridge struct {
	upstream   *url.URL
	listener   net.Listener
	cmd        *exec.Cmd
	localURL   string
	localUser  string
	localPass  string
	timeout    time.Duration
	insecure   bool
	closed     atomic.Bool
	lastUsedNS atomic.Int64
	closeOnce  sync.Once
}

// isHTTPSProxyURL 判断代理地址是否需要桥接为本地 HTTP 代理。
func isHTTPSProxyURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(u.Scheme, "https") && u.Host != ""
}

// bridgeHTTPSProxyURL 返回指向本地 HTTP 代理桥的 URL，桥后端连接真实 HTTPS 代理。
func bridgeHTTPSProxyURL(raw string, timeout time.Duration) (string, error) {
	if !isHTTPSProxyURL(raw) {
		return raw, nil
	}
	if timeout <= 0 {
		timeout = defaultHTTPSProxyBridgeDialTO
	}
	key := raw + "\x00" + timeout.String()

	httpsProxyBridgeMu.Lock()
	if bridge := httpsProxyBridgeCache[key]; bridge != nil && !bridge.closed.Load() {
		bridge.markUsed()
		localURL := bridge.localURL
		httpsProxyBridgeMu.Unlock()
		return localURL, nil
	}
	httpsProxyBridgeMu.Unlock()

	bridge, err := newHTTPSProxyBridge(raw, timeout)
	if err != nil {
		return "", err
	}

	httpsProxyBridgeMu.Lock()
	if existing := httpsProxyBridgeCache[key]; existing != nil && !existing.closed.Load() {
		_ = bridge.Close()
		existing.markUsed()
		localURL := existing.localURL
		httpsProxyBridgeMu.Unlock()
		return localURL, nil
	}
	httpsProxyBridgeCache[key] = bridge
	httpsProxyBridgeMu.Unlock()

	go reapHTTPSProxyBridge(key, bridge)
	return bridge.localURL, nil
}

// newHTTPSProxyBridge 启动只监听本机的 HTTP 代理桥。
func newHTTPSProxyBridge(raw string, timeout time.Duration) (*httpsProxyBridge, error) {
	if enablePythonHTTPSProxyBridge {
		if bridge, err := newPythonHTTPSProxyBridge(raw, timeout); err == nil {
			return bridge, nil
		}
	}
	return newGoHTTPSProxyBridge(raw, timeout)
}

// newGoHTTPSProxyBridge 启动 Go 实现的本地代理桥，作为系统 TLS 桥不可用时的回退。
func newGoHTTPSProxyBridge(raw string, timeout time.Duration) (*httpsProxyBridge, error) {
	upstream, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("代理地址解析失败: %w", err)
	}
	if !strings.EqualFold(upstream.Scheme, "https") || upstream.Host == "" {
		return nil, fmt.Errorf("仅 HTTPS 代理需要代理桥")
	}
	if timeout <= 0 {
		timeout = defaultHTTPSProxyBridgeDialTO
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("监听本地代理桥失败: %w", err)
	}
	localUser, err := randomBridgeToken()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	localPass, err := randomBridgeToken()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	local := &url.URL{
		Scheme: "http",
		User:   url.UserPassword(localUser, localPass),
		Host:   listener.Addr().String(),
	}
	bridge := &httpsProxyBridge{
		upstream:  upstream,
		listener:  listener,
		localURL:  local.String(),
		localUser: localUser,
		localPass: localPass,
		timeout:   timeout,
	}
	bridge.markUsed()
	go bridge.serve()
	return bridge, nil
}

// reapHTTPSProxyBridge 清理长期空闲的本地代理桥，避免开发模式反复检测后泄漏端口。
func reapHTTPSProxyBridge(key string, bridge *httpsProxyBridge) {
	ticker := time.NewTicker(httpsProxyBridgeReapInterval)
	defer ticker.Stop()
	for range ticker.C {
		if bridge.closed.Load() {
			return
		}
		lastUsed := time.Unix(0, bridge.lastUsedNS.Load())
		if time.Since(lastUsed) < httpsProxyBridgeIdleTTL {
			continue
		}
		_ = bridge.Close()
		httpsProxyBridgeMu.Lock()
		if httpsProxyBridgeCache[key] == bridge {
			delete(httpsProxyBridgeCache, key)
		}
		httpsProxyBridgeMu.Unlock()
		return
	}
}

// Close 关闭代理桥监听器，不主动中断已建立的隧道。
func (b *httpsProxyBridge) Close() error {
	var err error
	b.closeOnce.Do(func() {
		b.closed.Store(true)
		if b.listener != nil {
			err = b.listener.Close()
		}
		if b.cmd != nil && b.cmd.Process != nil {
			_ = b.cmd.Process.Kill()
		}
	})
	return err
}

// markUsed 记录代理桥最近使用时间。
func (b *httpsProxyBridge) markUsed() {
	b.lastUsedNS.Store(time.Now().UnixNano())
}

// serve 接收本地 HTTP 代理请求并转发到上游 HTTPS 代理。
func (b *httpsProxyBridge) serve() {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			return
		}
		b.markUsed()
		go b.handleConn(conn)
	}
}

// handleConn 处理单个本地 HTTP 代理连接。
func (b *httpsProxyBridge) handleConn(local net.Conn) {
	defer local.Close()
	_ = local.SetDeadline(time.Now().Add(b.timeout))

	reader := bufio.NewReader(local)
	req, err := stdhttp.ReadRequest(reader)
	if err != nil {
		return
	}
	defer req.Body.Close()
	if !b.authorized(req) {
		writeProxyBridgeError(local, stdhttp.StatusProxyAuthRequired, "Proxy Authentication Required")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	if strings.EqualFold(req.Method, stdhttp.MethodConnect) {
		b.handleConnect(ctx, local, reader, req.Host)
		return
	}
	b.handlePlainHTTP(ctx, local, req)
}

// authorized 校验本地代理桥的随机认证，避免本机其他进程直接复用开放代理。
func (b *httpsProxyBridge) authorized(req *stdhttp.Request) bool {
	header := req.Header.Get("Proxy-Authorization")
	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(header, prefix)))
	if err != nil {
		return false
	}
	return string(raw) == b.localUser+":"+b.localPass
}

// handleConnect 转发标准 CONNECT 隧道请求。
func (b *httpsProxyBridge) handleConnect(ctx context.Context, local net.Conn, reader *bufio.Reader, target string) {
	upstream, err := b.dialUpstreamTunnel(ctx, target)
	if err != nil {
		writeProxyBridgeError(local, stdhttp.StatusBadGateway, "Bad Gateway")
		return
	}
	defer upstream.Close()

	if _, err := io.WriteString(local, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if buffered := reader.Buffered(); buffered > 0 {
		data := make([]byte, buffered)
		if _, err := io.ReadFull(reader, data); err == nil {
			_, _ = upstream.Write(data)
		}
	}
	_ = local.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})
	copyBoth(local, upstream)
}

// handlePlainHTTP 支持本地代理收到明文 HTTP 请求时通过上游 CONNECT 转发。
func (b *httpsProxyBridge) handlePlainHTTP(ctx context.Context, local net.Conn, req *stdhttp.Request) {
	target, err := bridgeTargetAddr(req)
	if err != nil {
		writeProxyBridgeError(local, stdhttp.StatusBadRequest, "Bad Request")
		return
	}
	upstream, err := b.dialUpstreamTunnel(ctx, target)
	if err != nil {
		writeProxyBridgeError(local, stdhttp.StatusBadGateway, "Bad Gateway")
		return
	}
	defer upstream.Close()

	if err := writeOriginHTTPRequest(upstream, req); err != nil {
		return
	}
	_ = local.SetDeadline(time.Time{})
	_ = upstream.SetDeadline(time.Time{})
	_, _ = io.Copy(local, upstream)
}

// dialUpstreamTunnel 与上游 HTTPS 代理建立 TLS 连接并发起 CONNECT。
func (b *httpsProxyBridge) dialUpstreamTunnel(ctx context.Context, target string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: b.timeout}
	upstreamHost := b.upstreamHost()
	raw, err := dialer.DialContext(ctx, "tcp", upstreamHost)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = raw.SetDeadline(deadline)
	}

	tlsConn := utls.UClient(raw, b.clientTLSConfig(), utls.HelloIOS_Auto, false, false, false)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("HTTPS 代理握手失败: %w", err)
	}

	if err := b.writeUpstreamConnect(tlsConn, target); err != nil {
		_ = tlsConn.Close()
		return nil, err
	}
	resp, err := stdhttp.ReadResponse(bufio.NewReader(tlsConn), &stdhttp.Request{Method: stdhttp.MethodConnect})
	if err != nil {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("CONNECT 响应解析失败: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != stdhttp.StatusOK {
		_ = tlsConn.Close()
		return nil, fmt.Errorf("代理 CONNECT 失败: %s", resp.Status)
	}
	_ = tlsConn.SetDeadline(time.Time{})
	return tlsConn, nil
}

// clientTLSConfig 返回连接上游 HTTPS 代理的 TLS 配置。
func (b *httpsProxyBridge) clientTLSConfig() *utls.Config {
	return &utls.Config{
		ServerName:         b.upstream.Hostname(),
		InsecureSkipVerify: b.insecure,
	}
}

// upstreamHost 返回带默认端口的上游 HTTPS 代理地址。
func (b *httpsProxyBridge) upstreamHost() string {
	if b.upstream.Port() != "" {
		return b.upstream.Host
	}
	return net.JoinHostPort(b.upstream.Hostname(), "443")
}

// writeUpstreamConnect 写入发给上游 HTTPS 代理的 CONNECT 请求和认证头。
func (b *httpsProxyBridge) writeUpstreamConnect(conn net.Conn, target string) error {
	var request strings.Builder
	request.WriteString("CONNECT ")
	request.WriteString(target)
	request.WriteString(" HTTP/1.1\r\nHost: ")
	request.WriteString(target)
	request.WriteString("\r\n")
	if b.upstream.User != nil {
		password, _ := b.upstream.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(b.upstream.User.Username() + ":" + password))
		request.WriteString("Proxy-Authorization: Basic ")
		request.WriteString(token)
		request.WriteString("\r\n")
	}
	request.WriteString("\r\n")
	_, err := conn.Write([]byte(request.String()))
	return err
}

// bridgeTargetAddr 返回明文 HTTP 请求对应的目标 host:port。
func bridgeTargetAddr(req *stdhttp.Request) (string, error) {
	host := req.URL.Host
	if host == "" {
		host = req.Host
	}
	if host == "" {
		return "", fmt.Errorf("请求缺少目标主机")
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host, nil
	}
	port := "80"
	if strings.EqualFold(req.URL.Scheme, "https") {
		port = "443"
	}
	return net.JoinHostPort(host, port), nil
}

// writeOriginHTTPRequest 将代理请求改写为源站请求后写入上游隧道。
func writeOriginHTTPRequest(dst io.Writer, req *stdhttp.Request) error {
	path := req.URL.RequestURI()
	if path == "" {
		path = "/"
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	if _, err := fmt.Fprintf(dst, "%s %s HTTP/1.1\r\nHost: %s\r\n", req.Method, path, host); err != nil {
		return err
	}
	for key, values := range req.Header {
		if strings.EqualFold(key, "Proxy-Authorization") || strings.EqualFold(key, "Proxy-Connection") {
			continue
		}
		for _, value := range values {
			if _, err := fmt.Fprintf(dst, "%s: %s\r\n", key, value); err != nil {
				return err
			}
		}
	}
	if _, err := io.WriteString(dst, "\r\n"); err != nil {
		return err
	}
	if req.Body != nil {
		_, err := io.Copy(dst, req.Body)
		return err
	}
	return nil
}

// copyBoth 双向复制两个隧道连接。
func copyBoth(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(left, right)
		_ = left.Close()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(right, left)
		_ = right.Close()
	}()
	wg.Wait()
}

// writeProxyBridgeError 返回本地代理桥错误响应。
func writeProxyBridgeError(conn net.Conn, statusCode int, statusText string) {
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 %d %s\r\nContent-Length: 0\r\n\r\n", statusCode, statusText)
}

// randomBridgeToken 生成本地代理桥随机认证片段。
func randomBridgeToken() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成本地代理桥认证失败: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
