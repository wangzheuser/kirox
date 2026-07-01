package storage

import "testing"

func TestRegistrationConfigAcceptsTempMailPlusProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{"tempmail_plus"}}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(tempmail_plus) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "tempmail_plus" {
		t.Fatalf("EmailProviders=%#v, want tempmail_plus", got.EmailProviders)
	}
}
