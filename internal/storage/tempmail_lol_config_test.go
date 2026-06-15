package storage

import "testing"

func TestRegistrationConfigAcceptsTempMailLOLProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: "tempmail_lol"}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(tempmail_lol) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "tempmail_lol" {
		t.Fatalf("EmailProvider=%q, want tempmail_lol", got.EmailProvider)
	}
}
