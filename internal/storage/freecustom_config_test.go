package storage

import "testing"

func TestRegistrationConfigPersistsFreeCustomProvider(t *testing.T) {
	withTempStorageConfig(t, "")
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{"freecustom"}}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(freecustom) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "freecustom" {
		t.Fatalf("EmailProviders=%#v, want freecustom", got.EmailProviders)
	}
}
