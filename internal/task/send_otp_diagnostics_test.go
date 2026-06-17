package task

import "testing"

func TestParseSendOTPDiagnostics(t *testing.T) {
	errText := `注册被拦截: 请更换IP或稍后重试 [provider=tempmail_lol, domain=example.com, emailProxy=enabled, proxy=enabled]`
	got, ok := parseSendOTPDiagnostics(errText)
	if !ok {
		t.Fatalf("expected diagnostics to parse")
	}
	want := map[string]string{
		"provider":   "tempmail_lol",
		"domain":     "example.com",
		"emailProxy": "enabled",
		"proxy":      "enabled",
	}
	for key, expected := range want {
		if got[key] != expected {
			t.Fatalf("diagnostic %s=%q, want %q (all=%v)", key, got[key], expected, got)
		}
	}
}

func TestSendOTPDiagnosticsSummary(t *testing.T) {
	items := []map[string]string{
		{"provider": "tempmail_lol", "domain": "a.test", "emailProxy": "enabled", "proxy": "enabled"},
		{"provider": "tempmail_lol", "domain": "a.test", "emailProxy": "enabled", "proxy": "enabled"},
		{"provider": "emailnator", "domain": "gmail.com", "emailProxy": "direct", "proxy": "enabled"},
	}
	got := sendOTPDiagnosticsSummary(items)
	for _, want := range []string{"provider=tempmail_lol:2", "domain=a.test:2", "emailProxy=enabled:2", "proxy=enabled:3"} {
		if !containsText(got, want) {
			t.Fatalf("summary missing %q: %s", want, got)
		}
	}
}
