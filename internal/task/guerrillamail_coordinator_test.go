package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestGuerrillaMailCoordinatorDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProviders: []string{"guerrillamail"}})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("guerrillamail should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsGuerrillaMailProvider(t *testing.T) {
	service := &taskFakeTempEmailService{address: "reuse@guerrillamailblock.com"}
	pool := reusableEmailPool{}
	pool.put(reusableEmailCandidate{provider: "guerrillamail", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("guerrillamail")
	if !ok {
		t.Fatalf("expected guerrillamail reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("guerrillamail", &cfg, candidate)
	if !applied || address != service.address {
		t.Fatalf("guerrillamail reusable candidate not applied, address=%q applied=%v", address, applied)
	}
	if cfg.TempEmailService != service {
		t.Fatalf("guerrillamail should reuse TempEmailService")
	}
}
