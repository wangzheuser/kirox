package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSmailProCreateAndWaitForCode(t *testing.T) {
	var apiURL string
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/payload" {
			t.Fatalf("unexpected app path %s", r.URL.Path)
		}
		target := r.URL.Query().Get("url")
		if !strings.HasPrefix(target, apiURL) {
			t.Fatalf("payload url=%q, want api base %q", target, apiURL)
		}
		payload := strings.TrimPrefix(strings.TrimPrefix(target, apiURL), "/")
		if email := r.URL.Query().Get("email"); email != "" {
			payload += "|" + email
		}
		if mid := r.URL.Query().Get("mid"); mid != "" {
			payload += "|" + mid
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer app.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/create":
			if r.URL.Query().Get("payload") != "create" {
				t.Fatalf("create payload=%q", r.URL.Query().Get("payload"))
			}
			_, _ = w.Write([]byte(`{"email":"kiro@smail.test","expired_at":1782380081}`))
		case "/inbox":
			if r.URL.Query().Get("payload") != "inbox|kiro@smail.test" {
				t.Fatalf("inbox payload=%q", r.URL.Query().Get("payload"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": []map[string]interface{}{{"mid": "m1", "subject": "Verify"}},
			})
		case "/message":
			if r.URL.Query().Get("payload") != "message|kiro@smail.test|m1" {
				t.Fatalf("message payload=%q", r.URL.Query().Get("payload"))
			}
			_, _ = w.Write([]byte(`{"body":"Your AWS verification code is 123456","textSubject":"Verify"}`))
		default:
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
	}))
	defer api.Close()
	apiURL = api.URL

	oldApp, oldAPI := smailProAppBaseURL, smailProAPIBaseURL
	smailProAppBaseURL, smailProAPIBaseURL = app.URL, api.URL
	t.Cleanup(func() { smailProAppBaseURL, smailProAPIBaseURL = oldApp, oldAPI })

	service := NewSmailProService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError: %v", err)
	}
	if address != "kiro@smail.test" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode: %v", err)
	}
	if code != "123456" {
		t.Fatalf("code=%q", code)
	}
}

func TestTempMailboxCreateAndWaitForCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<meta name="csrf-token" content="csrf123">`))
		case "/get_messages":
			if r.FormValue("_token") != "csrf123" {
				t.Fatalf("token=%q", r.FormValue("_token"))
			}
			_, _ = w.Write([]byte(`{"status":true,"mailbox":"kiro@tempmailbox.test","email_token":"tok","messages":[{"id":"m1","from":"AWS","from_email":"no-reply@example.com","subject":"Verify"}]}`))
		case "/view/m1":
			_, _ = w.Write([]byte(`Your AWS verification code is 654321`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBase := tempMailboxBaseURL
	tempMailboxBaseURL = server.URL
	t.Cleanup(func() { tempMailboxBaseURL = oldBase })

	service := NewTempMailboxService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError: %v", err)
	}
	if address != "kiro@tempmailbox.test" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode: %v", err)
	}
	if code != "654321" {
		t.Fatalf("code=%q", code)
	}
}
