package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withMailTMTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldBase := mailTMAPIBaseURL
	mailTMAPIBaseURL = server.URL
	t.Cleanup(func() { mailTMAPIBaseURL = oldBase })
	return server.URL
}

func TestMailTMCreateRegistersAccountAndToken(t *testing.T) {
	var createdAddress string
	var tokenAddress string
	withMailTMTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/domains":
			if r.URL.Query().Get("page") != "1" {
				t.Fatalf("domains should request page=1, got %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hydra:member": []map[string]interface{}{
					{"domain": "inactive.example", "isActive": false},
					{"domain": "web-library.net", "isActive": true},
				},
			})
		case "/accounts":
			if r.Method != http.MethodPost {
				t.Fatalf("accounts method = %s, want POST", r.Method)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			createdAddress = body["address"]
			if !strings.HasSuffix(createdAddress, "@web-library.net") {
				t.Fatalf("created address = %q, want web-library.net", createdAddress)
			}
			if body["password"] == "" {
				t.Fatalf("password should be generated")
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"address": createdAddress})
		case "/token":
			if r.Method != http.MethodPost {
				t.Fatalf("token method = %s, want POST", r.Method)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			tokenAddress = body["address"]
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "jwt-token"})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewMailTMService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if address == "" || address != createdAddress || address != tokenAddress {
		t.Fatalf("address=%q created=%q token=%q", address, createdAddress, tokenAddress)
	}
	if service.GetAddress() != address {
		t.Fatalf("GetAddress()=%q, want %q", service.GetAddress(), address)
	}
}

func TestMailTMWaitForCodeReadsHydraMessagesAndText(t *testing.T) {
	withMailTMTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/messages") && r.Header.Get("Authorization") != "Bearer jwt-token" {
			t.Fatalf("Authorization=%q, want Bearer jwt-token", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/messages":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hydra:member": []map[string]interface{}{
					{"@id": "/messages/msg-1", "from": map[string]string{"address": "no-reply@signin.aws"}, "subject": "Verify your AWS Builder ID"},
				},
			})
		case "/messages/msg-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      "msg-1",
				"from":    map[string]string{"address": "no-reply@signin.aws"},
				"subject": "Verify your AWS Builder ID",
				"text":    "Your verification code is 123456. It expires soon.",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewMailTMService("")
	service.address = "test@web-library.net"
	service.token = "jwt-token"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "123456" {
		t.Fatalf("code=%q, want 123456", code)
	}
}

func TestMailTMWaitForCodeDoesNotMutateMailGWBaseURL(t *testing.T) {
	oldMailGWBase := mailGWAPIBaseURL
	mailGWAPIBaseURL = "https://mailgw.example.invalid"
	t.Cleanup(func() { mailGWAPIBaseURL = oldMailGWBase })

	withMailTMTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if mailGWAPIBaseURL != "https://mailgw.example.invalid" {
			t.Fatalf("mail.tm polling should not mutate mailGWAPIBaseURL, got %q", mailGWAPIBaseURL)
		}
		switch r.URL.Path {
		case "/messages":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"hydra:member": []map[string]interface{}{
					{"id": "msg-1", "from": map[string]string{"address": "no-reply@signin.aws"}, "subject": "Verify your AWS Builder ID"},
				},
			})
		case "/messages/msg-1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":      "msg-1",
				"from":    map[string]string{"address": "no-reply@signin.aws"},
				"subject": "Verify your AWS Builder ID",
				"text":    "Code: 987654",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewMailTMService("")
	service.address = "test@web-library.net"
	service.token = "jwt-token"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "987654" {
		t.Fatalf("code=%q, want 987654", code)
	}
}
