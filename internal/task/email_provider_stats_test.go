package task

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"reg_go/internal/email"
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

func TestCreateTempEmailPreferringSuccessfulDomainsRetriesUntilHistoricalSuccessDomain(t *testing.T) {
	isolateEmailProviderStatsStorage(t)
	storage.ResetEmailProviderStats()
	if err := storage.RecordEmailProviderRegistrationSuccess("blinkbox", "ok@fontdle.com"); err != nil {
		t.Fatalf("RecordEmailProviderRegistrationSuccess returned error: %v", err)
	}

	addresses := []string{"first@blocked.test", "second@FontDle.COM"}
	attempts := 0
	service, address, err := createTempEmailPreferringSuccessfulDomains(context.Background(), "blinkbox", "BlinkBoxApp", 1, 1, func() (email.TempEmailService, string, error) {
		addr := addresses[attempts]
		attempts++
		return &taskFakeTempEmailService{address: addr}, addr, nil
	})
	if err != nil {
		t.Fatalf("createTempEmailPreferringSuccessfulDomains returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if address != "second@FontDle.COM" {
		t.Fatalf("address = %q, want second@FontDle.COM", address)
	}
	if service.GetAddress() != address {
		t.Fatalf("service address = %q, want %q", service.GetAddress(), address)
	}
}

func TestCreateTempEmailPreferringSuccessfulDomainsIgnoresLowSampleSuccessDomains(t *testing.T) {
	isolateEmailProviderStatsStorage(t)
	storage.ResetEmailProviderStats()
	for _, emailAddr := range []string{
		"weak@beatnesia.my.id",
		"ok1@fontdle.com",
		"ok2@fontdle.com",
		"ok3@fontdle.com",
	} {
		if err := storage.RecordEmailProviderRegistrationSuccess("blinkbox", emailAddr); err != nil {
			t.Fatalf("RecordEmailProviderRegistrationSuccess(%q) returned error: %v", emailAddr, err)
		}
	}

	addresses := []string{"first@beatnesia.my.id", "second@fontdle.com"}
	attempts := 0
	_, address, err := createTempEmailPreferringSuccessfulDomains(context.Background(), "blinkbox", "BlinkBoxApp", 1, 1, func() (email.TempEmailService, string, error) {
		addr := addresses[attempts]
		attempts++
		return &taskFakeTempEmailService{address: addr}, addr, nil
	})
	if err != nil {
		t.Fatalf("createTempEmailPreferringSuccessfulDomains returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if address != "second@fontdle.com" {
		t.Fatalf("address = %q, want second@fontdle.com", address)
	}
}

func TestCreateTempEmailPreferringSuccessfulDomainsFailsAfterTenMisses(t *testing.T) {
	isolateEmailProviderStatsStorage(t)
	storage.ResetEmailProviderStats()
	if err := storage.RecordEmailProviderRegistrationSuccess("blinkbox", "ok@fontdle.com"); err != nil {
		t.Fatalf("RecordEmailProviderRegistrationSuccess returned error: %v", err)
	}

	attempts := 0
	_, address, err := createTempEmailPreferringSuccessfulDomains(context.Background(), "blinkbox", "BlinkBoxApp", 1, 1, func() (email.TempEmailService, string, error) {
		attempts++
		addr := fmt.Sprintf("miss%d@beatnesia.my.id", attempts)
		return &taskFakeTempEmailService{address: addr}, addr, nil
	})
	if err == nil || !strings.Contains(err.Error(), "未命中历史成功域名") {
		t.Fatalf("expected historical-success-domain miss error, got address=%q err=%v", address, err)
	}
	if attempts != 10 {
		t.Fatalf("attempts = %d, want 10", attempts)
	}
	if address != "" {
		t.Fatalf("address should be empty after domain miss error, got %q", address)
	}
}

func TestCreateTempEmailPreferringSuccessfulDomainsDoesNotFilterWithoutHistory(t *testing.T) {
	isolateEmailProviderStatsStorage(t)
	storage.ResetEmailProviderStats()

	attempts := 0
	_, address, err := createTempEmailPreferringSuccessfulDomains(context.Background(), "smailpro", "SmailPro", 1, 1, func() (email.TempEmailService, string, error) {
		attempts++
		return &taskFakeTempEmailService{address: "first@blocked.test"}, "first@blocked.test", nil
	})
	if err != nil {
		t.Fatalf("createTempEmailPreferringSuccessfulDomains returned error: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if address != "first@blocked.test" {
		t.Fatalf("address = %q, want first@blocked.test", address)
	}
}

func TestCreateTempEmailPreferringSuccessfulDomainsUsesBlinkBoxDefaultFontdleDomain(t *testing.T) {
	isolateEmailProviderStatsStorage(t)
	storage.ResetEmailProviderStats()

	addresses := []string{"first@beatnesia.my.id", "second@fontdle.com"}
	attempts := 0
	_, address, err := createTempEmailPreferringSuccessfulDomains(context.Background(), "blinkbox", "BlinkBoxApp", 1, 1, func() (email.TempEmailService, string, error) {
		addr := addresses[attempts]
		attempts++
		return &taskFakeTempEmailService{address: addr}, addr, nil
	})
	if err != nil {
		t.Fatalf("createTempEmailPreferringSuccessfulDomains returned error: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if address != "second@fontdle.com" {
		t.Fatalf("address = %q, want second@fontdle.com", address)
	}
}
