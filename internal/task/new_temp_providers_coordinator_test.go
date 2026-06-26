package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
)

func TestNewZeroConfigProvidersDoNotRequireOutlookAccounts(t *testing.T) {
	for _, provider := range []string{"mailcatch", "tempmailo", "minuteinbox", "smailpro", "tempmailbox", "generator_email", "mailtowin", "mail2me", "pickmemail", "maximail", "emlpro", "freeml", "emlhub", "emltmp", "mailpwr", "tenmail", "dropmail_me", "mimimail", "pickmail", "spymail", "yomail", "tmio_bltiwd", "tmio_wnbaldwy", "tmio_bwmyga", "tmio_ozsaip", "gonebox", "openinbox", "blinkbox"} {
		t.Run(provider, func(t *testing.T) {
			Manager.mu.Lock()
			Manager.running = false
			Manager.mu.Unlock()

			result := StartTask(StartTaskRequest{Count: 1, EmailProviders: []string{provider}})
			if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
				t.Fatalf("%s should not require Outlook accounts, got error %q", provider, errText)
			}
			StopTask(true)
		})
	}
}

func TestReusableEmailSupportsNewZeroConfigProviders(t *testing.T) {
	for _, provider := range []string{"mailcatch", "tempmailo", "minuteinbox", "smailpro", "tempmailbox", "generator_email", "mailtowin", "mail2me", "pickmemail", "maximail", "emlpro", "freeml", "emlhub", "emltmp", "mailpwr", "tenmail", "dropmail_me", "mimimail", "pickmail", "spymail", "yomail", "tmio_bltiwd", "tmio_wnbaldwy", "tmio_bwmyga", "tmio_ozsaip", "gonebox", "openinbox", "blinkbox"} {
		t.Run(provider, func(t *testing.T) {
			pool := reusableEmailPool{}
			service := &taskFakeTempEmailService{address: "reuse@example.test"}
			pool.put(reusableEmailCandidate{provider: provider, address: service.address, tempEmailService: service})

			candidate, ok := pool.take(provider)
			if !ok {
				t.Fatalf("expected reusable candidate for %s", provider)
			}
			var cfg core.Config
			address, applied := applyReusableEmailCandidate(provider, &cfg, candidate)
			if !applied || address != service.address || cfg.TempEmailService != service {
				t.Fatalf("candidate not applied for %s, address=%q applied=%v", provider, address, applied)
			}
		})
	}
}
