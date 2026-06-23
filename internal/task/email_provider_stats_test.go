package task

import (
	"testing"

	"reg_go/internal/storage"
)

func isolateEmailProviderStatsStorage(t *testing.T) {
	t.Helper()
	tempRoot := t.TempDir()
	t.Setenv("APPDATA", tempRoot)
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)
	t.Setenv("USERPROFILE", tempRoot)
}

func TestRecordEmailProviderOTPStatFromAttemptResult(t *testing.T) {
	isolateEmailProviderStatsStorage(t)
	storage.ResetEmailProviderStats()

	recordEmailProviderOTPStat("mailgw", map[string]interface{}{
		"status":      "failed",
		"email":       "user@example.com",
		"otpReceived": true,
	})
	recordEmailProviderOTPStat("mailgw", map[string]interface{}{
		"status": "failed",
		"email":  "no-otp@example.com",
	})

	stats := storage.GetEmailProviderStats()
	if len(stats) != 1 {
		t.Fatalf("expected one provider stat, got %#v", stats)
	}
	if stats[0].Provider != "mailgw" || stats[0].OTPReceivedCount != 1 || stats[0].RegistrationSuccessCount != 0 {
		t.Fatalf("unexpected stats after otp attempt: %#v", stats)
	}
}

func TestRecordEmailProviderSuccessStatFromFinalResult(t *testing.T) {
	isolateEmailProviderStatsStorage(t)
	storage.ResetEmailProviderStats()

	recordEmailProviderSuccessStat("mailgw", map[string]interface{}{
		"status": "success",
		"email":  "User@Example.COM",
	})
	recordEmailProviderSuccessStat("mailgw", map[string]interface{}{
		"status": "failed",
		"email":  "failed@example.com",
	})

	stats := storage.GetEmailProviderStats()
	if len(stats) != 1 {
		t.Fatalf("expected one provider stat, got %#v", stats)
	}
	if stats[0].Provider != "mailgw" || stats[0].RegistrationSuccessCount != 1 || stats[0].SuccessDomains["@example.com"] != 1 {
		t.Fatalf("unexpected stats after success result: %#v", stats)
	}
}
