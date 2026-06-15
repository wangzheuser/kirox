package core

import (
	"testing"

	"reg_go/internal/email"
)

type fakeTempEmailService struct {
	address     string
	createCalls int
}

func (s *fakeTempEmailService) Create() string {
	s.createCalls++
	return s.address
}

func (s *fakeTempEmailService) WaitForCode(int, int) (string, error) {
	return "", nil
}

func (s *fakeTempEmailService) GetAddress() string {
	return s.address
}

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

func TestNewConfigDefaultsOutlookScopeToIMAP(t *testing.T) {
	cfg := NewConfig()

	if cfg.OutlookScope != OutlookScopeIMAP {
		t.Fatalf("默认 Outlook 读取方式应为 imap: got %q", cfg.OutlookScope)
	}
}

func TestUseOutlookGraph(t *testing.T) {
	cfg := &Config{OutlookScope: OutlookScopeGraph}

	if !cfg.UseOutlookGraph() {
		t.Fatalf("OutlookScope=graph 时应启用 Graph 读取")
	}
}

func TestStep3EmailUsesReusableTempEmailService(t *testing.T) {
	service := &fakeTempEmailService{address: "reuse@example.com"}
	r := &Registrar{Cfg: &Config{TempEmailService: service}}

	if err := r.Step3Email(); err != nil {
		t.Fatalf("Step3Email returned error: %v", err)
	}
	if r.Email != "reuse@example.com" {
		t.Fatalf("Step3Email email = %q, want reuse@example.com", r.Email)
	}
	if r.EmailSvc != service {
		t.Fatalf("Step3Email should use reusable service")
	}
	if service.createCalls != 0 {
		t.Fatalf("existing reusable service should not create a new mailbox, createCalls=%d", service.createCalls)
	}
}

func TestStep3EmailCreatesEmailnatorService(t *testing.T) {
	oldFactory := newEmailnatorTempEmailService
	service := &fakeTempEmailService{address: "generated@gmail.com"}
	newEmailnatorTempEmailService = func(proxyURL string) email.TempEmailService {
		if proxyURL != "http://proxy.example:8080" {
			t.Fatalf("proxyURL = %q, want http://proxy.example:8080", proxyURL)
		}
		return service
	}
	t.Cleanup(func() { newEmailnatorTempEmailService = oldFactory })

	r := &Registrar{Cfg: &Config{EmailProvider: "emailnator", EmailProxy: "http://proxy.example:8080"}}
	if err := r.Step3Email(); err != nil {
		t.Fatalf("Step3Email returned error: %v", err)
	}
	if r.Email != "generated@gmail.com" {
		t.Fatalf("Step3Email email = %q, want generated@gmail.com", r.Email)
	}
	if r.EmailSvc != service {
		t.Fatalf("Step3Email should bind generated Emailnator service")
	}
	if service.createCalls != 1 {
		t.Fatalf("Emailnator service should create mailbox once, createCalls=%d", service.createCalls)
	}
}
