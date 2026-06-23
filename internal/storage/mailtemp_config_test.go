package storage

import "testing"

func TestRegistrationConfigAcceptsMailTempProvider(t *testing.T) {
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{"mailtemp"}}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(mailtemp) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "mailtemp" {
		t.Fatalf("EmailProviders=%#v, want mailtemp", got.EmailProviders)
	}
}
