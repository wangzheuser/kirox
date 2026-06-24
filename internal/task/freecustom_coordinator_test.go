package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestFreeCustomDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProviders: []string{"freecustom"}})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("freecustom should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestFreeCustomFixedDomainDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProviders: []string{"fce_areueally"}})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") || strings.Contains(errText, "未知邮箱提供商") {
		t.Fatalf("fixed FreeCustom provider should be zero-config, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsFreeCustomProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@areueally.info"}
	pool.put(reusableEmailCandidate{provider: "freecustom", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("freecustom")
	if !ok {
		t.Fatalf("expected freecustom reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("freecustom", &cfg, candidate)
	if !applied || address != service.address || cfg.TempEmailService != service {
		t.Fatalf("freecustom reusable candidate not applied, address=%q applied=%v", address, applied)
	}
}

func TestReusableEmailSupportsFreeCustomFixedDomainProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@areueally.info"}
	pool.put(reusableEmailCandidate{provider: "fce_areueally", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("fce_areueally")
	if !ok {
		t.Fatalf("expected fce_areueally reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("fce_areueally", &cfg, candidate)
	if !applied || address != service.address || cfg.TempEmailService != service {
		t.Fatalf("fixed FreeCustom reusable candidate not applied, address=%q applied=%v", address, applied)
	}
}
