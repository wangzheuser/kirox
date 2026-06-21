package storage

import "testing"

func TestRegistrationConfigAcceptsTempMailPlusProvider(t *testing.T) {
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: "tempmail_plus"}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(tempmail_plus) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "tempmail_plus" {
		t.Fatalf("EmailProvider=%q, want tempmail_plus", got.EmailProvider)
	}
}
