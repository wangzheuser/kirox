package core

import (
	"errors"
	"strings"
	"testing"
)

func TestSendOTPFailureContextIncludesSafeDiagnosticsWithoutPageTiming(t *testing.T) {
	cfg := &Config{EmailProvider: "tempmail_lol", EmailProxy: "http://127.0.0.1:7890", Proxy: "http://127.0.0.1:7890", AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: -7, TimeZoneSet: true}
	msg := sendOTPFailureContext(cfg, "user@example.com")
	for _, want := range []string{"provider=tempmail_lol", "domain=example.com", "emailProxy=enabled", "proxy=enabled", "acceptLanguage=en-US", "i18next=en-US", "timeZone=-7"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("diagnostic context missing %q: %s", want, msg)
		}
	}
	for _, forbidden := range []string{"user@example.com", "page" + "Stay", "time" + "OnPage"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("diagnostic context should not include %q: %s", forbidden, msg)
		}
	}
}

func TestSendOTPFailureContextHandlesBlankProxyWithoutPageTiming(t *testing.T) {
	cfg := &Config{EmailProvider: "emailnator"}
	msg := sendOTPFailureContext(cfg, "invalid-email")
	for _, want := range []string{"provider=emailnator", "domain=<unknown>", "emailProxy=direct", "proxy=direct", "acceptLanguage=zh-CN", "i18next=zh-CN", "timeZone=8"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("diagnostic context missing %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "page"+"Stay") || strings.Contains(msg, "time"+"OnPage") {
		t.Fatalf("diagnostic context should not include page timing: %s", msg)
	}
}

func TestFormatErrorPreservesSendOTPDiagnosticsWithoutPageTiming(t *testing.T) {
	r := &Registrar{}
	errText := `send-otp 失败 (400): {"errorCode":"BLOCKED","message":"Request was blocked by TES."} [provider=tempmail_lol, domain=example.com, emailProxy=enabled, proxy=enabled, acceptLanguage=en-US, i18next=en-US, timeZone=-7]`
	got := r.formatError("SendOTP", errors.New(errText))
	for _, want := range []string{"注册被拦截", "provider=tempmail_lol", "domain=example.com", "acceptLanguage=en-US", "timeZone=-7", "i18next=en-US"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatted send-otp error missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "page"+"Stay") || strings.Contains(got, "time"+"OnPage") {
		t.Fatalf("formatted send-otp error should not include page timing: %s", got)
	}
}
