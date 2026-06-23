package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestMailTMDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProviders: []string{"mailtm"}})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("mailtm should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsMailTMProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@web-library.net"}
	pool.put(reusableEmailCandidate{provider: "mailtm", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("mailtm")
	if !ok {
		t.Fatalf("expected mailtm reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("mailtm", &cfg, candidate)
	if !applied || address != service.address {
		t.Fatalf("mailtm reusable candidate not applied, address=%q applied=%v", address, applied)
	}
	if cfg.TempEmailService != service {
		t.Fatalf("mailtm should reuse TempEmailService")
	}
}

func TestMailTMSendOTP400DoesNotTriggerKillSwitch(t *testing.T) {
	if isKillSwitchError("send-otp 失败 (400): domain rejected", "mailtm") {
		t.Fatalf("mailtm send-otp 400 should be treated as single mailbox failure")
	}
	if !isKillSwitchError("注册请求被拦截 BLOCKED", "mailtm") {
		t.Fatalf("mailtm BLOCKED/IP risk should still trigger kill switch")
	}
}
