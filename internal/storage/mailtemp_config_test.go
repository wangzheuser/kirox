package storage

import "testing"

func TestRegistrationConfigAcceptsMailTempProvider(t *testing.T) {
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: "mailtemp"}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(mailtemp) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "mailtemp" {
		t.Fatalf("EmailProvider=%q, want mailtemp", got.EmailProvider)
	}
}
