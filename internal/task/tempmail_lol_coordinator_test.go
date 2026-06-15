package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestTempMailLOLDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProvider: "tempmail_lol"})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("tempmail_lol should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsTempMailLOLProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@random.example"}
	pool.put(reusableEmailCandidate{provider: "tempmail_lol", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("tempmail_lol")
	if !ok {
		t.Fatalf("expected tempmail_lol reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("tempmail_lol", &cfg, candidate)
	if !applied || address != service.address || cfg.TempEmailService != service {
		t.Fatalf("tempmail_lol reusable candidate not applied, address=%q applied=%v", address, applied)
	}
}

func TestTempMailLOLSendOTP400DoesNotTriggerKillSwitch(t *testing.T) {
	if isKillSwitchError("send-otp 失败 (400): domain rejected", "tempmail_lol") {
		t.Fatalf("tempmail_lol send-otp 400 should be treated as single mailbox failure")
	}
	if !isKillSwitchError("注册请求被拦截 BLOCKED", "tempmail_lol") {
		t.Fatalf("tempmail_lol BLOCKED/IP risk should still trigger kill switch")
	}
}
