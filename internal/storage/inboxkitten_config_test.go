package storage

import "testing"

func TestRegistrationConfigAcceptsInboxKittenProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{"inboxkitten"}}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(inboxkitten) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "inboxkitten" {
		t.Fatalf("EmailProviders=%#v, want inboxkitten", got.EmailProviders)
	}
}
