package storage

import "testing"

func TestRegistrationConfigPersistsDropMailProvider(t *testing.T) {
	withTempStorageConfig(t, "")
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: "dropmail"}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(dropmail) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "dropmail" {
		t.Fatalf("EmailProvider=%q, want dropmail", got.EmailProvider)
	}
}
