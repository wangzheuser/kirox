package email

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClientWithProxyBoundsIdleConnections(t *testing.T) {
	client := httpClientWithProxy("http://127.0.0.1:9200", 3*time.Second)
	if client.Timeout != 3*time.Second {
		t.Fatalf("client timeout = %s, want 3s", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}
	if transport.IdleConnTimeout != 5*time.Second {
		t.Fatalf("idle timeout = %s, want 5s", transport.IdleConnTimeout)
	}
	if transport.MaxIdleConns != 16 || transport.MaxIdleConnsPerHost != 1 {
		t.Fatalf("unexpected idle connection limits: max=%d perHost=%d", transport.MaxIdleConns, transport.MaxIdleConnsPerHost)
	}
	if transport.DisableKeepAlives {
		t.Fatalf("email client should allow short polling reuse while bounding idle connections")
	}
}

func TestDoWithProxyHTTPClientClosesIdleConnectionsWhenBodyCloses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	resp, err := doWithProxyHTTPClient("", 3*time.Second, func(client *http.Client) (*http.Response, error) {
		return client.Get(server.URL)
	})
	if err != nil {
		t.Fatalf("request returned error: %v", err)
	}
	if _, ok := resp.Body.(*closeIdleReadCloser); !ok {
		t.Fatalf("response body should close idle connections, got %T", resp.Body)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
}
