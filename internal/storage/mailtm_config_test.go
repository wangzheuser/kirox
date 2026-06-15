package storage

import "testing"

func TestRegistrationConfigAcceptsMailTMProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{
		Count:         1,
		SuccessTarget: 1,
		Concurrency:   1,
		Delay:         0,
		RetryCount:    0,
		OTPTimeout:    120,
		EmailProvider: "mailtm",
	}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(mailtm) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "mailtm" {
		t.Fatalf("EmailProvider=%q, want mailtm", got.EmailProvider)
	}
}
