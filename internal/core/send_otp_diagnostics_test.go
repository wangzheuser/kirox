package core

import (
	"errors"
	"strings"
	"testing"
)

func TestSendOTPFailureContextIncludesSafeDiagnostics(t *testing.T) {
	cfg := &Config{EmailProvider: "tempmail_lol", EmailProxy: "http://127.0.0.1:7890", Proxy: "http://127.0.0.1:7890", PageStayMinMs: 5000, PageStayMaxMs: 8000, AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: -7, TimeZoneSet: true}
	msg := sendOTPFailureContext(cfg, "user@example.com", 6400)
	for _, want := range []string{"provider=tempmail_lol", "domain=example.com", "emailProxy=enabled", "proxy=enabled", "pageStay=5000-8000ms", "timeOnPage=6400ms", "acceptLanguage=en-US", "i18next=en-US", "timeZone=-7"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("diagnostic context missing %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "user@example.com") {
		t.Fatalf("diagnostic context should not include full email address: %s", msg)
	}
}

func TestSendOTPFailureContextHandlesBlankProxy(t *testing.T) {
	cfg := &Config{EmailProvider: "emailnator"}
	msg := sendOTPFailureContext(cfg, "invalid-email", 0)
	for _, want := range []string{"provider=emailnator", "domain=<unknown>", "emailProxy=direct", "proxy=direct", "pageStay=0-0ms", "timeOnPage=0ms", "acceptLanguage=zh-CN", "i18next=zh-CN", "timeZone=8"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("diagnostic context missing %q: %s", want, msg)
		}
	}
}

func TestFormatErrorPreservesSendOTPDiagnostics(t *testing.T) {
	r := &Registrar{}
	errText := `send-otp 失败 (400): {"errorCode":"BLOCKED","message":"Request was blocked by TES."} [provider=tempmail_lol, domain=example.com, emailProxy=enabled, proxy=enabled, pageStay=5000-8000ms, timeOnPage=6400ms, acceptLanguage=en-US, i18next=en-US, timeZone=-7]`
	got := r.formatError("SendOTP", errors.New(errText))
	for _, want := range []string{"注册被拦截", "provider=tempmail_lol", "domain=example.com", "pageStay=5000-8000ms", "timeOnPage=6400ms", "acceptLanguage=en-US", "timeZone=-7", "acceptLanguage=en-US", "i18next=en-US", "timeZone=-7"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted send-otp error missing %q: %s", want, got)
		}
	}
}
