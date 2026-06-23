package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestMailTempCoordinatorDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProviders: []string{"mailtemp"}})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("mailtemp should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsMailTempProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@himacreative.id"}
	pool.put(reusableEmailCandidate{provider: "mailtemp", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("mailtemp")
	if !ok {
		t.Fatalf("expected mailtemp reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("mailtemp", &cfg, candidate)
	if !applied || address != service.address || cfg.TempEmailService != service {
		t.Fatalf("mailtemp reusable candidate not applied, address=%q applied=%v", address, applied)
	}
}
