package storage

import "testing"

func TestRegistrationConfigAcceptsGuerrillaMailProvider(t *testing.T) {
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{"guerrillamail"}}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(guerrillamail) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "guerrillamail" {
		t.Fatalf("EmailProviders=%#v, want guerrillamail", got.EmailProviders)
	}
}
