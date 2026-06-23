package storage

import "testing"

func TestRegistrationConfigAcceptsMailTMProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{
		Count:          1,
		SuccessTarget:  1,
		Concurrency:    1,
		Delay:          0,
		RetryCount:     0,
		OTPTimeout:     120,
		EmailProviders: []string{"mailtm"},
	}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(mailtm) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "mailtm" {
		t.Fatalf("EmailProviders=%#v, want mailtm", got.EmailProviders)
	}
}

func TestDefaultRegistrationConfigUsesSixtySecondOTPTimeout(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := GetRegistrationConfig()
	if cfg.OTPTimeout != 60 {
		t.Fatalf("default OTPTimeout=%d, want 60", cfg.OTPTimeout)
	}
}

func TestRegistrationConfigPreservesCustomOTPTimeout(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := defaultRegistrationConfig()
	cfg.OTPTimeout = 90
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig: %v", err)
	}
	got := GetRegistrationConfig()
	if got.OTPTimeout != 90 {
		t.Fatalf("custom OTPTimeout=%d, want 90", got.OTPTimeout)
	}
}
