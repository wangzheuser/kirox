package storage

import "testing"

func TestRegistrationConfigPersistsNewZeroConfigProviders(t *testing.T) {
	providers := []string{"mailcatch", "tempmailo", "minuteinbox", "smailpro", "tempmailbox", "generator_email", "mailtowin", "mail2me", "pickmemail", "maximail", "emlpro", "freeml", "emlhub", "emltmp", "mailpwr", "tenmail", "dropmail_me", "mimimail", "pickmail", "spymail", "yomail", "tmio_bltiwd", "tmio_wnbaldwy", "tmio_bwmyga", "tmio_ozsaip"}
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			withTempStorageConfig(t, "")
			cfg := RegistrationConfig{Count: 1, SuccessTarget: 1, Concurrency: 1, Delay: 0, EmailProviders: []string{provider}}
			if err := SetRegistrationConfig(cfg); err != nil {
				t.Fatalf("SetRegistrationConfig(%s) error: %v", provider, err)
			}
			if got := GetRegistrationConfig(); len(got.EmailProviders) != 1 || got.EmailProviders[0] != provider {
				t.Fatalf("EmailProviders=%#v, want %s", got.EmailProviders, provider)
			}
		})
	}
}
