package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withInboxKittenTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldBase := inboxKittenAPIBaseURL
	inboxKittenAPIBaseURL = server.URL
	t.Cleanup(func() { inboxKittenAPIBaseURL = oldBase })
	return server.URL
}

func TestInboxKittenCreateGeneratesInboxKittenAddress(t *testing.T) {
	service := NewInboxKittenService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if !strings.HasSuffix(address, "@inboxkitten.com") {
		t.Fatalf("address=%q, want inboxkitten.com domain", address)
	}
	if service.GetAddress() != address {
		t.Fatalf("GetAddress=%q, want %q", service.GetAddress(), address)
	}
	local := strings.TrimSuffix(address, "@inboxkitten.com")
	if len(local) < 8 || strings.ContainsAny(local, "@ ") {
		t.Fatalf("local part %q is not a generated mailbox name", local)
	}
}

func TestInboxKittenWaitForCodeReadsListAndHTML(t *testing.T) {
	var sawList bool
	var sawHTML bool
	withInboxKittenTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/list":
			sawList = true
			if r.URL.Query().Get("recipient") != "kirotest" {
				t.Fatalf("recipient=%q, want kirotest", r.URL.Query().Get("recipient"))
			}
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{
					"timestamp": 1710000000,
					"message": map[string]interface{}{
						"headers": map[string]interface{}{
							"from":    "AWS Builder ID <no-reply@signin.aws>",
							"subject": "Verify your AWS Builder ID email address",
						},
					},
					"storage": map[string]interface{}{"region": "sg", "key": "msg-key"},
				},
			})
		case "/getHtml":
			sawHTML = true
			if r.URL.Query().Get("region") != "sg" || r.URL.Query().Get("key") != "msg-key" {
				t.Fatalf("detail query=%s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`<html><body>Your verification code is <b>135790</b>.</body></html>`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewInboxKittenService("")
	service.address = "kirotest@inboxkitten.com"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "135790" {
		t.Fatalf("code=%q, want 135790", code)
	}
	if !sawList || !sawHTML {
		t.Fatalf("expected list and html endpoints to be called, sawList=%v sawHTML=%v", sawList, sawHTML)
	}
}
