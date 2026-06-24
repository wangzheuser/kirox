package email

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFreeCustomCreateUsesDomainsAPIAndAuth(t *testing.T) {
	oldIx := freeCustomDomainIx
	freeCustomDomainIx = 0
	t.Cleanup(func() { freeCustomDomainIx = oldIx })

	var authSeen, domainsSeen, mailboxSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			authSeen = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"test-token"}`))
		case "/api/domains":
			domainsSeen = true
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("domains auth=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":[{"domain":"ditapi.info","tier":"free"},{"domain":"paid.example","tier":"pro"}]}`))
		case "/api/public-mailbox":
			mailboxSeen = true
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("mailbox auth=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewFreeCustomService("")
	service.baseURL = server.URL
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError error: %v", err)
	}
	if !strings.HasSuffix(address, "@ditapi.info") {
		t.Fatalf("address=%q, want ditapi.info", address)
	}
	if !authSeen || !domainsSeen || !mailboxSeen {
		t.Fatalf("expected auth/domains/mailbox calls, auth=%v domains=%v mailbox=%v", authSeen, domainsSeen, mailboxSeen)
	}
}

func TestFreeCustomFixedDomainServiceUsesOnlyConfiguredDomain(t *testing.T) {
	var domainsSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"test-token"}`))
		case "/api/domains":
			domainsSeen = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":[{"domain":"wrong.example","tier":"free"}]}`))
		case "/api/public-mailbox":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewFreeCustomFixedDomainService("", "ditpay.info")
	service.baseURL = server.URL
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError error: %v", err)
	}
	if !strings.HasSuffix(address, "@ditpay.info") {
		t.Fatalf("address=%q, want ditpay.info", address)
	}
	if domainsSeen {
		t.Fatalf("fixed-domain FreeCustom provider should not call dynamic domains API")
	}
}

func TestFreeCustomFixedDomainChannelsExposeCurrentFreeDomains(t *testing.T) {
	channels := FreeCustomFixedDomainChannels()
	if len(channels) != 3 {
		t.Fatalf("fixed domain channels=%d, want exactly 3 validated providers", len(channels))
	}
	seenProviders := map[string]bool{}
	seenDomains := map[string]bool{}
	for _, ch := range channels {
		if ch.Provider == "" || ch.Domain == "" || ch.Label == "" {
			t.Fatalf("channel has empty field: %#v", ch)
		}
		if seenProviders[ch.Provider] {
			t.Fatalf("duplicate provider %q", ch.Provider)
		}
		if seenDomains[ch.Domain] {
			t.Fatalf("duplicate domain %q", ch.Domain)
		}
		seenProviders[ch.Provider] = true
		seenDomains[ch.Domain] = true
	}
	for _, want := range []string{"areueally.info", "junkstopper.info", "ditpay.info"} {
		if !seenDomains[want] {
			t.Fatalf("missing fixed FreeCustom domain %q in %#v", want, channels)
		}
	}
}

func TestFreeCustomWaitForCodeReadsMessageDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/public-mailbox":
			if got := r.URL.Query().Get("fullMailboxId"); got != "kirotest@ditapi.info" {
				t.Fatalf("fullMailboxId=%q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("messageId") == "m1" {
				_, _ = w.Write([]byte(`{"success":true,"data":{"id":"m1","from":"no-reply@signin.aws","subject":"Verify your email","text":"Your verification code is 654321"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"data":[{"id":"m1","from":"no-reply@signin.aws","subject":"Verify your email"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := NewFreeCustomService("")
	service.baseURL = server.URL
	service.address = "kirotest@ditapi.info"
	service.token = "test-token"
	code, err := service.WaitForCode(2, 1)
	if err != nil {
		t.Fatalf("WaitForCode error: %v", err)
	}
	if code != "654321" {
		t.Fatalf("code=%q, want 654321", code)
	}
}
