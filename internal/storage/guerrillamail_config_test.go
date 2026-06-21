package storage

import "testing"

func TestRegistrationConfigAcceptsGuerrillaMailProvider(t *testing.T) {
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: "guerrillamail"}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(guerrillamail) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "guerrillamail" {
		t.Fatalf("EmailProvider=%q, want guerrillamail", got.EmailProvider)
	}
}
