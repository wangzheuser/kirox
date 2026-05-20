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

func TestRandomPageStayMsAllowsZeroRange(t *testing.T) {
	cfg := &Config{PageStayMinMs: 0, PageStayMaxMs: 0}

	if got := cfg.RandomPageStayMs(); got != 0 {
		t.Fatalf("0/0 页面停留配置应返回 0: got %d", got)
	}
}

func TestRandomPageStayMsAllowsFixedRange(t *testing.T) {
	cfg := &Config{PageStayMinMs: 2500, PageStayMaxMs: 2500}

	if got := cfg.RandomPageStayMs(); got != 2500 {
		t.Fatalf("固定页面停留配置应返回固定值: got %d", got)
	}
}

func TestRandomPageStayMsKeepsValueInRange(t *testing.T) {
	cfg := &Config{PageStayMinMs: 1200, PageStayMaxMs: 1800}

	for i := 0; i < 100; i++ {
		got := cfg.RandomPageStayMs()
		if got < cfg.PageStayMinMs || got > cfg.PageStayMaxMs {
			t.Fatalf("页面停留随机值越界: got %d, range [%d,%d]", got, cfg.PageStayMinMs, cfg.PageStayMaxMs)
		}
	}
}
