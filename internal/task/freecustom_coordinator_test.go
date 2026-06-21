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

	result := StartTask(StartTaskRequest{Count: 1, EmailProvider: "freecustom"})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("freecustom should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsFreeCustomProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@ditapi.info"}
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
