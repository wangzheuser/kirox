package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestDropMailDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProviders: []string{"dropmail"}})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("dropmail should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsDropMailProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@mailtowin.com"}
	pool.put(reusableEmailCandidate{provider: "dropmail", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("dropmail")
	if !ok {
		t.Fatalf("expected dropmail reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("dropmail", &cfg, candidate)
	if !applied || address != service.address || cfg.TempEmailService != service {
		t.Fatalf("dropmail reusable candidate not applied, address=%q applied=%v", address, applied)
	}
}
