package storage

import "testing"

func TestRegistrationConfigPersistsNewZeroConfigProviders(t *testing.T) {
	providers := []string{"mailcatch", "tempmailo", "generator_email", "mailtowin", "mail2me", "pickmemail", "maximail", "emlpro", "freeml", "emlhub", "emltmp", "mailpwr"}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			withTempStorageConfig(t, "")
			cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProvider: provider}
			if err := SetRegistrationConfig(cfg); err != nil {
				t.Fatalf("SetRegistrationConfig(%s) error: %v", provider, err)
			}
			if got := GetRegistrationConfig(); got.EmailProvider != provider {
				t.Fatalf("EmailProvider=%q, want %s", got.EmailProvider, provider)
			}
		})
	}
}
