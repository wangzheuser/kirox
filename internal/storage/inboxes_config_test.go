package storage

import "testing"

func TestRegistrationConfigAcceptsInboxesProvider(t *testing.T) {
	withTempStorageConfig(t, "")

	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: "inboxes"}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(inboxes) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "inboxes" {
		t.Fatalf("EmailProvider=%q, want inboxes", got.EmailProvider)
	}
}
