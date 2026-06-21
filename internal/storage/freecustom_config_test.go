package storage

import "testing"

func TestRegistrationConfigPersistsFreeCustomProvider(t *testing.T) {
	withTempStorageConfig(t, "")
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: "freecustom"}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(freecustom) error: %v", err)
	}
	if got := GetRegistrationConfig(); got.EmailProvider != "freecustom" {
		t.Fatalf("EmailProvider=%q, want freecustom", got.EmailProvider)
	}
}
