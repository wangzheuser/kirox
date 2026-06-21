package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withInboxesTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldBase := inboxesAPIBaseURL
	inboxesAPIBaseURL = server.URL
	t.Cleanup(func() { inboxesAPIBaseURL = oldBase })
}

func TestInboxesCreateSignsUpAndUsesDomainPool(t *testing.T) {
	var sawSignup bool
	var sawDomain bool
	withInboxesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/signup":
			sawSignup = true
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "access-token"})
		case "/domain":
			sawDomain = true
			if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Fatalf("Authorization=%q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"randomUser": "serveruser",
				"domains":    []map[string]string{{"qdn": "getairmail.com"}, {"qdn": "vomoto.com"}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewInboxesService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if !strings.HasSuffix(address, "@getairmail.com") && !strings.HasSuffix(address, "@vomoto.com") {
		t.Fatalf("address=%q, want Inboxes domain", address)
	}
	if service.GetAddress() != address || service.token != "access-token" {
		t.Fatalf("state not stored: address=%q token=%q", service.GetAddress(), service.token)
	}
	if !sawSignup || !sawDomain {
		t.Fatalf("signup/domain not called: signup=%v domain=%v", sawSignup, sawDomain)
	}
}

func TestInboxesCreateDoesNotUseNilRandomUser(t *testing.T) {
	withInboxesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/signup":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "access-token"})
		case "/domain":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"domains": []map[string]string{{"qdn": "getairmail.com"}},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewInboxesService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if strings.HasPrefix(address, "nil@") || strings.HasPrefix(address, "<nil>@") {
		t.Fatalf("address should not use missing randomUser as local part: %q", address)
	}
}

func TestInboxesWaitForCodeReadsInboxAndMessageDetail(t *testing.T) {
	withInboxesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization=%q", got)
		}
		switch r.URL.Path {
		case "/inbox/codex@getairmail.com":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"msgs": []map[string]interface{}{
					{"uid": "msg-1", "f": "AWS Builder ID <no-reply@signin.aws>", "s": "Verify your AWS Builder ID", "ph": "code inside"},
				},
			})
		case "/message/msg-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"uid":  "msg-1",
				"text": "Your verification code is 864209.",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewInboxesService("")
	service.address = "codex@getairmail.com"
	service.token = "access-token"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "864209" {
		t.Fatalf("code=%q, want 864209", code)
	}
}
