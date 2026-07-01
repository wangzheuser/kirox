package storage

import (
	"os"
	"strings"
	"sync"
	"testing"
)

func findEmailProviderStat(t *testing.T, stats []EmailProviderStat, provider string) EmailProviderStat {
	t.Helper()
	for _, stat := range stats {
		if stat.Provider == provider {
			return stat
		}
	}
	t.Fatalf("provider %q not found in stats: %#v", provider, stats)
	return EmailProviderStat{}
}

func TestEmailProviderStatsRecordAndPersist(t *testing.T) {
	withTempStorageConfig(t, "")

	if err := RecordEmailProviderOTPReceived(" emailnator "); err != nil {
		t.Fatalf("RecordEmailProviderOTPReceived returned error: %v", err)
	}
	if err := RecordEmailProviderOTPReceived("emailnator"); err != nil {
		t.Fatalf("RecordEmailProviderOTPReceived returned error: %v", err)
	}
	if err := RecordEmailProviderDomainAttempt("emailnator", "User@Example.COM"); err != nil {
		t.Fatalf("RecordEmailProviderDomainAttempt returned error: %v", err)
	}
	if err := RecordEmailProviderDomainAttempt("emailnator", "other@example.com"); err != nil {
		t.Fatalf("RecordEmailProviderDomainAttempt returned error: %v", err)
	}
	if err := RecordEmailProviderRegistrationSuccess("emailnator", "User@Example.COM"); err != nil {
		t.Fatalf("RecordEmailProviderRegistrationSuccess returned error: %v", err)
	}
	if err := RecordEmailProviderRegistrationSuccess("emailnator", "other@example.com"); err != nil {
		t.Fatalf("RecordEmailProviderRegistrationSuccess returned error: %v", err)
	}

	stat := findEmailProviderStat(t, GetEmailProviderStats(), "emailnator")
	if stat.OTPReceivedCount != 2 {
		t.Fatalf("OTPReceivedCount = %d, want 2", stat.OTPReceivedCount)
	}
	if stat.RegistrationSuccessCount != 2 {
		t.Fatalf("RegistrationSuccessCount = %d, want 2", stat.RegistrationSuccessCount)
	}
	if got := stat.SuccessDomains["@example.com"]; got != 2 {
		t.Fatalf("SuccessDomains[@example.com] = %d, want 2; map=%#v", got, stat.SuccessDomains)
	}
	if got := stat.DomainAttempts["@example.com"]; got != 2 {
		t.Fatalf("DomainAttempts[@example.com] = %d, want 2; map=%#v", got, stat.DomainAttempts)
	}
	if strings.TrimSpace(stat.UpdatedAt) == "" {
		t.Fatalf("UpdatedAt should be set")
	}

	// 重新读取应来自持久化文件，统计值保持不变。
	stat = findEmailProviderStat(t, GetEmailProviderStats(), "emailnator")
	if stat.OTPReceivedCount != 2 || stat.RegistrationSuccessCount != 2 || stat.SuccessDomains["@example.com"] != 2 || stat.DomainAttempts["@example.com"] != 2 {
		t.Fatalf("persisted stat mismatch: %+v", stat)
	}
}

func TestEmailProviderStatsRecordDomainAttemptAndReadLegacyFile(t *testing.T) {
	withTempStorageConfig(t, "")

	legacyJSON := `{"blinkbox":{"provider":"blinkbox","otpReceivedCount":1,"registrationSuccessCount":1,"successDomains":{"@fontdle.com":1},"updatedAt":"2026-07-01T00:00:00Z"}}`
	if err := os.WriteFile(emailProviderStatsFilePath(), []byte(legacyJSON), 0600); err != nil {
		t.Fatalf("write legacy stats: %v", err)
	}

	stat := findEmailProviderStat(t, GetEmailProviderStats(), "blinkbox")
	if stat.DomainAttempts == nil {
		t.Fatalf("legacy stats should initialize DomainAttempts to an empty map")
	}

	if err := RecordEmailProviderDomainAttempt("blinkbox", "user@FontDle.COM"); err != nil {
		t.Fatalf("RecordEmailProviderDomainAttempt returned error: %v", err)
	}

	stat = findEmailProviderStat(t, GetEmailProviderStats(), "blinkbox")
	if got := stat.DomainAttempts["@fontdle.com"]; got != 1 {
		t.Fatalf("DomainAttempts[@fontdle.com] = %d, want 1; stat=%+v", got, stat)
	}
	if got := stat.SuccessDomains["@fontdle.com"]; got != 1 {
		t.Fatalf("legacy SuccessDomains should be preserved, got %d", got)
	}
}

func TestEmailProviderStatsRejectInvalidProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	if err := RecordEmailProviderOTPReceived("invalid-provider"); err == nil {
		t.Fatalf("invalid provider should be rejected")
	}
	if len(GetEmailProviderStats()) != 0 {
		t.Fatalf("invalid provider should not create stats: %#v", GetEmailProviderStats())
	}
}

func TestEmailProviderStatsResetClearsAll(t *testing.T) {
	withTempStorageConfig(t, "")

	if err := RecordEmailProviderOTPReceived("mailgw"); err != nil {
		t.Fatalf("RecordEmailProviderOTPReceived returned error: %v", err)
	}
	if err := RecordEmailProviderRegistrationSuccess("mailgw", "ok@domain.test"); err != nil {
		t.Fatalf("RecordEmailProviderRegistrationSuccess returned error: %v", err)
	}

	if err := ResetEmailProviderStats(); err != nil {
		t.Fatalf("ResetEmailProviderStats returned error: %v", err)
	}
	if got := GetEmailProviderStats(); len(got) != 0 {
		t.Fatalf("stats should be empty after reset, got %#v", got)
	}
}

func TestEmailProviderStatsConcurrentRecording(t *testing.T) {
	withTempStorageConfig(t, "")

	const workers = 40
	var wg sync.WaitGroup
	wg.Add(workers * 2)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if err := RecordEmailProviderOTPReceived("mailtm"); err != nil {
				t.Errorf("RecordEmailProviderOTPReceived returned error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if err := RecordEmailProviderDomainAttempt("mailtm", "ok@Example.ORG"); err != nil {
				t.Errorf("RecordEmailProviderDomainAttempt returned error: %v", err)
			}
			if err := RecordEmailProviderRegistrationSuccess("mailtm", "ok@Example.ORG"); err != nil {
				t.Errorf("RecordEmailProviderRegistrationSuccess returned error: %v", err)
			}
		}()
	}
	wg.Wait()

	stat := findEmailProviderStat(t, GetEmailProviderStats(), "mailtm")
	if stat.OTPReceivedCount != workers {
		t.Fatalf("OTPReceivedCount = %d, want %d", stat.OTPReceivedCount, workers)
	}
	if stat.RegistrationSuccessCount != workers {
		t.Fatalf("RegistrationSuccessCount = %d, want %d", stat.RegistrationSuccessCount, workers)
	}
	if got := stat.SuccessDomains["@example.org"]; got != workers {
		t.Fatalf("SuccessDomains[@example.org] = %d, want %d", got, workers)
	}
	if got := stat.DomainAttempts["@example.org"]; got != workers {
		t.Fatalf("DomainAttempts[@example.org] = %d, want %d", got, workers)
	}
}
