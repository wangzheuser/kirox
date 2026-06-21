package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func withGuerrillaMailTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldBase := guerrillaMailAPIBaseURL
	guerrillaMailAPIBaseURL = server.URL + "/ajax.php"
	t.Cleanup(func() { guerrillaMailAPIBaseURL = oldBase })
}

func TestGuerrillaMailCreateStoresAddressAndSession(t *testing.T) {
	withGuerrillaMailTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("f") != "get_email_address" {
			t.Fatalf("f=%q, want get_email_address", r.URL.Query().Get("f"))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"email_addr": "abc@guerrillamailblock.com",
			"sid_token":  "sid-123",
		})
	})

	service := NewGuerrillaMailService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if address != "abc@guerrillamailblock.com" {
		t.Fatalf("address=%q", address)
	}
	if service.GetAddress() != address {
		t.Fatalf("GetAddress()=%q, want %q", service.GetAddress(), address)
	}
}

func TestGuerrillaMailWaitForCodeFetchesAWSMessage(t *testing.T) {
	withGuerrillaMailTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("f") {
		case "check_email":
			if r.URL.Query().Get("sid_token") != "sid-123" {
				t.Fatalf("sid_token=%q, want sid-123", r.URL.Query().Get("sid_token"))
			}
			if r.URL.Query().Get("seq") != "0" {
				t.Fatalf("seq=%q, want 0", r.URL.Query().Get("seq"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"list": []map[string]interface{}{
					{"mail_id": "42", "mail_from": "no-reply@signin.aws", "mail_subject": "Verify your AWS Builder ID"},
				},
			})
		case "fetch_email":
			if r.URL.Query().Get("email_id") != "42" {
				t.Fatalf("email_id=%q, want 42", r.URL.Query().Get("email_id"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"mail_id":      "42",
				"mail_from":    "no-reply@signin.aws",
				"mail_subject": "Verify your AWS Builder ID",
				"mail_body":    "<p>Your verification code is <b>123456</b>.</p>",
			})
		default:
			t.Fatalf("unexpected f=%q", r.URL.Query().Get("f"))
		}
	})

	service := NewGuerrillaMailService("")
	service.address = "abc@guerrillamailblock.com"
	service.sidToken = "sid-123"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "123456" {
		t.Fatalf("code=%q, want 123456", code)
	}
}
