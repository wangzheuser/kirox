package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withEmailnatorTestServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	oldBase := emailnatorBaseURL
	oldHome := emailnatorHomeURL
	oldGenerate := emailnatorGenerateURL
	oldMessages := emailnatorMessagesURL
	emailnatorBaseURL = server.URL
	emailnatorHomeURL = server.URL + "/"
	emailnatorGenerateURL = server.URL + "/generate-email"
	emailnatorMessagesURL = server.URL + "/message-list"
	t.Cleanup(func() {
		emailnatorBaseURL = oldBase
		emailnatorHomeURL = oldHome
		emailnatorGenerateURL = oldGenerate
		emailnatorMessagesURL = oldMessages
	})
	return server.URL
}

func TestEmailnatorCreateEmailRetriesUntilGmailAddressAndSendsXSRF(t *testing.T) {
	generateCalls := 0
	var gotXSRF string
	withEmailnatorTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "abc%3D"})
			_, _ = w.Write([]byte("<html>ok</html>"))
		case "/generate-email":
			generateCalls++
			gotXSRF = r.Header.Get("X-XSRF-TOKEN")
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				t.Fatalf("missing X-Requested-With header: %s", r.Header.Get("X-Requested-With"))
			}
			if generateCalls == 1 {
				_ = json.NewEncoder(w).Encode(map[string][]string{"email": {"bad@tmpmailtor.com"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string][]string{"email": {"good+abc@gmail.com"}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewEmailnatorService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if address != "good+abc@gmail.com" {
		t.Fatalf("address = %q, want good+abc@gmail.com", address)
	}
	if generateCalls != 2 {
		t.Fatalf("generate calls = %d, want 2", generateCalls)
	}
	if gotXSRF != "abc=" {
		t.Fatalf("XSRF header = %q, want abc=", gotXSRF)
	}
}

func TestEmailnatorCreateEmailSendsSessionCookiesWithXSRF(t *testing.T) {
	withEmailnatorTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "abc%3D"})
			http.SetCookie(w, &http.Cookie{Name: "gmailnator_session", Value: "session-1", HttpOnly: true})
			_, _ = w.Write([]byte("<html>ok</html>"))
		case "/generate-email":
			if r.Header.Get("X-XSRF-TOKEN") != "abc=" {
				w.WriteHeader(http.StatusTeapot)
				_, _ = w.Write([]byte("missing decoded xsrf"))
				return
			}
			if _, err := r.Cookie("XSRF-TOKEN"); err != nil {
				w.WriteHeader(419)
				_, _ = w.Write([]byte("missing xsrf cookie"))
				return
			}
			if cookie, err := r.Cookie("gmailnator_session"); err != nil || cookie.Value != "session-1" {
				w.WriteHeader(419)
				_, _ = w.Write([]byte("missing session cookie"))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string][]string{"email": {"good+abc@gmail.com"}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewEmailnatorService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if address != "good+abc@gmail.com" {
		t.Fatalf("address = %q, want good+abc@gmail.com", address)
	}
}

func TestEmailnatorWaitForCodeReadsMessageDetailHTML(t *testing.T) {
	withEmailnatorTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "XSRF-TOKEN", Value: "abc%3D"})
			_, _ = w.Write([]byte("<html>ok</html>"))
		case "/generate-email":
			_ = json.NewEncoder(w).Encode(map[string][]string{"email": {"good+abc@gmail.com"}})
		case "/message-list":
			var payload map[string]string
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["messageID"] == "m1" || payload["id"] == "m1" {
				_, _ = w.Write([]byte(`<html><body>Your AWS Builder ID verification code is <b>654321</b>.</body></html>`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"messageData": []map[string]string{
					{"messageID": "m1", "from": "no-reply@signin.aws", "subject": "AWS Builder ID verification"},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})

	service := NewEmailnatorService("")
	if _, err := service.CreateWithError(); err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}

	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "654321" {
		t.Fatalf("code = %q, want 654321", code)
	}
}

func TestNormalizeEmailnatorMessagesSupportsKnownResponseShapes(t *testing.T) {
	cases := []string{
		`{"messageData":[{"messageID":"m1"}]}`,
		`{"data":[{"id":"m2"}]}`,
		`{"messages":[{"id":"m3"}]}`,
		`{"data":{"messages":[{"id":"m4"}]}}`,
	}

	for _, raw := range cases {
		var payload interface{}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatal(err)
		}
		messages := normalizeEmailnatorMessages(payload)
		if len(messages) != 1 {
			t.Fatalf("normalizeEmailnatorMessages(%s) len = %d, want 1", raw, len(messages))
		}
		if id := emailnatorMessageID(messages[0]); !strings.HasPrefix(id, "m") {
			t.Fatalf("unexpected message id for %s: %q", raw, id)
		}
	}
}
