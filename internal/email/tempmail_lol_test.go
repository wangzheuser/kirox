package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withTempMailLOLTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldBase := tempMailLOLAPIBaseURL
	tempMailLOLAPIBaseURL = server.URL
	t.Cleanup(func() { tempMailLOLAPIBaseURL = oldBase })
	return server.URL
}

func TestTempMailLOLCreateStoresAddressAndToken(t *testing.T) {
	withTempMailLOLTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/inbox/create" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"address": "user@random.example",
			"token":   "inbox-token",
		})
	})

	service := NewTempMailLOLService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if address != "user@random.example" || service.GetAddress() != address || service.token != "inbox-token" {
		t.Fatalf("address/token not stored: address=%q got=%q token=%q", address, service.GetAddress(), service.token)
	}
}

func TestTempMailLOLWaitForCodeReadsEmailsAndHTML(t *testing.T) {
	withTempMailLOLTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/inbox" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("token") != "inbox-token" {
			t.Fatalf("token query=%q, want inbox-token", r.URL.Query().Get("token"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"expired": false,
			"emails": []map[string]interface{}{
				{
					"id":      "msg-1",
					"from":    "no-reply@signin.aws",
					"subject": "Verify your AWS Builder ID",
					"body":    "<html><body>Your verification code is <b>246810</b>.</body></html>",
				},
			},
		})
	})

	service := NewTempMailLOLService("")
	service.address = "user@random.example"
	service.token = "inbox-token"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "246810" {
		t.Fatalf("code=%q, want 246810", code)
	}
}
