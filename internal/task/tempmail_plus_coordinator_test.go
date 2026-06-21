package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestTempMailPlusCoordinatorDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProvider: "tempmail_plus"})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("tempmail_plus should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsTempMailPlusProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@fexpost.com"}
	pool.put(reusableEmailCandidate{provider: "tempmail_plus", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("tempmail_plus")
	if !ok {
		t.Fatalf("expected tempmail_plus reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("tempmail_plus", &cfg, candidate)
	if !applied || address != service.address || cfg.TempEmailService != service {
		t.Fatalf("tempmail_plus reusable candidate not applied, address=%q applied=%v", address, applied)
	}
}
