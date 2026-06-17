package task

import (
	"strings"
	"testing"
)

func TestFailureDiagnosisHighlightsSendOTPWindControl(t *testing.T) {
	got := failureDiagnosisSummary(map[string]int{"IP/指纹风控": 6, "验证码发送失败": 2}, 10, 0)
	if got == "" {
		t.Fatalf("expected diagnosis summary for wind-control heavy failures")
	}
	for _, want := range []string{"send-otp", "TES/BLOCKED", "代理/IP/指纹"} {
		if !containsText(got, want) {
			t.Fatalf("diagnosis summary missing %q: %s", want, got)
		}
	}
}

func TestFailureDiagnosisHighlightsOTPDomainRejected(t *testing.T) {
	got := failureDiagnosisSummary(map[string]int{"验证码发送失败": 8}, 10, 0)
	if got == "" {
		t.Fatalf("expected diagnosis summary for send-otp 400 failures")
	}
	for _, want := range []string{"send-otp", "邮箱域名", "邮件提供商"} {
		if !containsText(got, want) {
			t.Fatalf("diagnosis summary missing %q: %s", want, got)
		}
	}
}

func TestFailureDiagnosisEmptyWhenSuccessful(t *testing.T) {
	if got := failureDiagnosisSummary(map[string]int{"验证码发送失败": 1}, 10, 1); got != "" {
		t.Fatalf("successful batch should not print failure diagnosis, got %q", got)
	}
}

func containsText(s, sub string) bool {
	return strings.Contains(s, sub)
}
