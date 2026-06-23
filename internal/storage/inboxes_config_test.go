package storage

import "testing"

func TestRegistrationConfigAcceptsInboxesProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{"inboxes"}}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(inboxes) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "inboxes" {
		t.Fatalf("EmailProviders=%#v, want inboxes", got.EmailProviders)
	}
}
