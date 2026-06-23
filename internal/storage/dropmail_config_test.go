package storage

import "testing"

func TestRegistrationConfigPersistsDropMailProvider(t *testing.T) {
	withTempStorageConfig(t, "")
	cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{"dropmail"}}
	if err := SetRegistrationConfig(cfg); err != nil {
		t.Fatalf("SetRegistrationConfig(dropmail) error: %v", err)
	}
	if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != "dropmail" {
		t.Fatalf("EmailProviders=%#v, want dropmail", got.EmailProviders)
	}
}
