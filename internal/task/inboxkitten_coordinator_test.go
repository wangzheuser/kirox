package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestInboxKittenDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProvider: "inboxkitten"})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("inboxkitten should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsInboxKittenProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@inboxkitten.com"}
	pool.put(reusableEmailCandidate{provider: "inboxkitten", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("inboxkitten")
	if !ok {
		t.Fatalf("expected inboxkitten reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("inboxkitten", &cfg, candidate)
	if !applied || address != service.address || cfg.TempEmailService != service {
		t.Fatalf("inboxkitten reusable candidate not applied, address=%q applied=%v", address, applied)
	}
}
