package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withMailGWTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldBase := mailGWAPIBaseURL
	mailGWAPIBaseURL = server.URL
	t.Cleanup(func() { mailGWAPIBaseURL = oldBase })
	return server.URL
}

func TestMailGWCreateRegistersAccountAndToken(t *testing.T) {
	var createdAddress string
	var tokenAddress string
	withMailGWTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/domains":
			if r.URL.Query().Get("page") != "1" {
				t.Fatalf("domains should request page=1, got %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{
				{"domain": "inactive.example", "isActive": false},
				{"domain": "oakon.com", "isActive": true},
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
			if !strings.HasSuffix(createdAddress, "@oakon.com") {
				t.Fatalf("created address = %q, want oakon.com", createdAddress)
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

	service := NewMailGWService("")
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

func TestMailGWWaitForCodeReadsHydraMessagesAndHTML(t *testing.T) {
	withMailGWTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" && r.Header.Get("Authorization") != "Bearer jwt-token" {
			t.Fatalf("Authorization=%q, want Bearer jwt-token", r.Header.Get("Authorization"))
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
				"html":    []string{"<html><body>Your verification code is <b>654321</b>.</body></html>"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewMailGWService("")
	service.address = "test@oakon.com"
	service.token = "jwt-token"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "654321" {
		t.Fatalf("code=%q, want 654321", code)
	}
}

func TestNormalizeMailGWMessagesSupportsCommonResponseShapes(t *testing.T) {
	cases := []string{
		`[{"id":"a"}]`,
		`{"hydra:member":[{"id":"b"}]}`,
		`{"messages":[{"id":"c"}]}`,
		`{"data":{"messages":[{"id":"d"}]}}`,
	}
	for _, raw := range cases {
		var payload interface{}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatal(err)
		}
		messages := normalizeMailGWMessages(payload)
		if len(messages) != 1 {
			t.Fatalf("normalizeMailGWMessages(%s) len = %d, want 1", raw, len(messages))
		}
	}
}
