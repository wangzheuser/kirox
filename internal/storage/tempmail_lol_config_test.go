package storage

import "testing"

func TestRegistrationConfigAcceptsTempMailLOLProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{"tempmail_lol"}}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(tempmail_lol) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "tempmail_lol" {
		t.Fatalf("EmailProviders=%#v, want tempmail_lol", got.EmailProviders)
	}
}
