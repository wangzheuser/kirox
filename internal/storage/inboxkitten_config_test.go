package storage

import "testing"

func TestRegistrationConfigAcceptsInboxKittenProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: "inboxkitten"}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(inboxkitten) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "inboxkitten" {
		t.Fatalf("EmailProvider=%q, want inboxkitten", got.EmailProvider)
	}
}
