package core

import "testing"

func TestMaxHTTPRetriesDisablesSameNodeRetryForProxyPool(t *testing.T) {
	r := &Registrar{Cfg: &Config{Proxy: "https://node.test:pass@proxy.example.com:443", ProxySwitchable: true}}

	if got := r.maxHTTPRetries(); got != 0 {
		t.Fatalf("代理池节点不应在同一节点上重复传输重试: got %d", got)
	}
}

func TestMaxHTTPRetriesKeepsCompatibilityForProxyFromPool(t *testing.T) {
	r := &Registrar{Cfg: &Config{Proxy: "https://node.test:pass@proxy.example.com:443", ProxyFromPool: true}}

	if got := r.maxHTTPRetries(); got != 0 {
		t.Fatalf("旧代理池标记应继续禁用同节点传输重试: got %d", got)
	}
}

func TestMaxHTTPRetriesKeepsDefaultForFixedProxy(t *testing.T) {
	r := &Registrar{Cfg: &Config{Proxy: "https://user:pass@proxy.example.com:443"}}

	if got := r.maxHTTPRetries(); got != 2 {
		t.Fatalf("固定代理应保留默认重试次数: got %d", got)
	}
}
