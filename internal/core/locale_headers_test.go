package core

import "testing"

func TestFetchD2CTokenUsesConfiguredBrowserLocale(t *testing.T) {
	cfg := NewConfig()
	cfg.AcceptLanguage = "en-US,en;q=0.9"
	cfg.I18Next = "en-US"
	cfg.TimeZone = -7
	cfg.TimeZoneSet = true
	reg := NewRegistrar(cfg)
	headers := reg.BuildD2CTokenHeaders("https://profile.aws.amazon.com", "https://profile.aws.amazon.com/")
	if got := headers["Accept-Language"]; got != "en-US,en;q=0.9" {
		t.Fatalf("D2C Accept-Language=%q, want en-US,en;q=0.9", got)
	}
}

func TestProfileInitDocumentHeadersUseConfiguredBrowserLocale(t *testing.T) {
	cfg := NewConfig()
	cfg.AcceptLanguage = "en-US,en;q=0.9"
	cfg.I18Next = "en-US"
	cfg.TimeZone = -7
	cfg.TimeZoneSet = true
	reg := NewRegistrar(cfg)
	headers := reg.BuildDocumentHeaders()
	if got := headers["Accept-Language"]; got != "en-US,en;q=0.9" {
		t.Fatalf("document Accept-Language=%q, want en-US,en;q=0.9", got)
	}
	if got := headers["sec-ch-ua"]; got != reg.Identity.SecUA {
		t.Fatalf("document sec-ch-ua mismatch")
	}
}
