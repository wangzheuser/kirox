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

func TestStep3EmailCreatesMailGWService(t *testing.T) {
	oldFactory := newMailGWTempEmailService
	service := &fakeTempEmailService{address: "user@oakon.com"}
	newMailGWTempEmailService = func(proxyURL string) email.TempEmailService {
		if proxyURL != "http://mail-proxy" {
			t.Fatalf("proxyURL=%q, want http://mail-proxy", proxyURL)
		}
		return service
	}
	t.Cleanup(func() { newMailGWTempEmailService = oldFactory })

	r := &Registrar{Cfg: &Config{EmailProvider: "mailgw", EmailProxy: "http://mail-proxy"}}
	if err := r.Step3Email(); err != nil {
		t.Fatalf("Step3Email returned error: %v", err)
	}
	if r.Email != "user@oakon.com" || r.EmailSvc != service {
		t.Fatalf("Step3Email should bind generated mail.gw service, email=%q svc=%#v", r.Email, r.EmailSvc)
	}
	if service.createCalls != 1 {
		t.Fatalf("mail.gw service should create mailbox once, createCalls=%d", service.createCalls)
	}
}

func TestRegistrarUsesFingerprintKeyInsteadOfLocalClashProxy(t *testing.T) {
	cfgA := &Config{Proxy: "http://127.0.0.1:7890", FingerprintKey: "clash:node-a"}
	cfgB := &Config{Proxy: "http://127.0.0.1:7890", FingerprintKey: "clash:node-b"}

	regA := NewRegistrar(cfgA)
	regB := NewRegistrar(cfgB)

	if regA.Identity == nil || regB.Identity == nil {
		t.Fatalf("registrars should have identities")
	}
	if regA.Identity.CanvasHash == regB.Identity.CanvasHash && regA.Identity.GPUModel == regB.Identity.GPUModel && regA.Identity.Screen.Width == regB.Identity.Screen.Width && regA.Identity.Screen.Height == regB.Identity.Screen.Height {
		t.Fatalf("different fingerprint keys should not reuse the same hardware identity")
	}
}

func TestStep3EmailUsesGraphRegistrationEmailWhenPresent(t *testing.T) {
	acc := &email.OutlookAccount{Email: "alias@outlook.jp", RegistrationEmail: "actual@hotmail.com"}
	r := &Registrar{Cfg: &Config{UseOutlook: true, OutlookScope: OutlookScopeGraph, OutlookAccount: acc}}

	if err := r.Step3Email(); err != nil {
		t.Fatalf("Step3Email returned error: %v", err)
	}
	if r.Email != "actual@hotmail.com" {
		t.Fatalf("Graph mode should register with verified Graph address, got %q", r.Email)
	}
}
