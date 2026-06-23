package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMailCatchCreateAndWaitForCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/list/kirotestbox":
			_, _ = w.Write([]byte(`<a class="message" data-id="m1" href="/api/data/kirotestbox/m1">Verify your AWS Builder ID email address</a>`))
		case "/api/data/kirotestbox/m1":
			_, _ = w.Write([]byte(`From: no-reply@signin.aws<br>Verification code: <b>123456</b>`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewMailCatchService("")
	service.baseURL = server.URL
	service.localGenerator = func() string { return "kirotestbox" }

	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError error: %v", err)
	}
	if address != "kirotestbox@mailcatch.com" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(2, 1)
	if err != nil {
		t.Fatalf("WaitForCode error: %v", err)
	}
	if code != "123456" {
		t.Fatalf("code=%q, want 123456", code)
	}
}

func TestTempMailoCreatesWithAntiforgeryTokenAndReadsInbox(t *testing.T) {
	var tokenSeen, mailboxPosted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			if r.Method == http.MethodGet {
				http.SetCookie(w, &http.Cookie{Name: ".AspNetCore.Antiforgery.test", Value: "cookie-token"})
				_, _ = w.Write([]byte(`<input name="__RequestVerificationToken" type="hidden" value="form-token" />`))
				return
			}
			if r.Method == http.MethodPost {
				mailboxPosted = true
				if r.Header.Get("RequestVerificationToken") != "form-token" {
					t.Fatalf("missing antiforgery token")
				}
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode post: %v", err)
				}
				if payload["mail"] != "user@forexzig.com" {
					t.Fatalf("mail payload=%q", payload["mail"])
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"from":"no-reply@signin.aws","subject":"Verify your AWS Builder ID email address","body":"Your code is 234567"}]`))
				return
			}
		case "/changemail":
			tokenSeen = r.Header.Get("RequestVerificationToken") == "form-token"
			_, _ = w.Write([]byte(`user@forexzig.com`))
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	service := NewTempMailoService("")
	service.baseURL = server.URL
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError error: %v", err)
	}
	if address != "user@forexzig.com" || !tokenSeen {
		t.Fatalf("address=%q tokenSeen=%v", address, tokenSeen)
	}
	code, err := service.WaitForCode(2, 1)
	if err != nil {
		t.Fatalf("WaitForCode error: %v", err)
	}
	if code != "234567" || !mailboxPosted {
		t.Fatalf("code=%q mailboxPosted=%v", code, mailboxPosted)
	}
}

func TestGeneratorEmailCreateAndWaitForCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`<html><body>No dynamic domains in this fixture</body></html>`))
		case r.URL.Path == "/check_adres_validation3.php":
			_, _ = w.Write([]byte(`{"status":"good","uptime":"100"}`))
		case strings.Contains(r.URL.Path, "kiroprobe@email-temp.com") || strings.Contains(r.URL.Path, "/email-temp.com/kiroprobe"):
			_, _ = w.Write([]byte(`<span id="email_ch_text">kiroprobe@email-temp.com</span><div>Verify your AWS Builder ID email address</div><b>345678</b>`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewGeneratorEmailService("")
	service.baseURL = server.URL
	service.localGenerator = func() string { return "kiroprobe" }
	service.domains = []string{"email-temp.com"}

	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError error: %v", err)
	}
	if address != "kiroprobe@email-temp.com" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(2, 1)
	if err != nil {
		t.Fatalf("WaitForCode error: %v", err)
	}
	if code != "345678" {
		t.Fatalf("code=%q, want 345678", code)
	}
}

func TestGeneratorEmailCreateUsesDynamicHomeDomainsBeforeFallback(t *testing.T) {
	var validatedDomain string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`<div class="tt-suggestion"><p onclick="change_dropdown_list(this.innerHTML)" id="dynamic-mail.com">dynamic-mail.com</p></div>`))
		case r.URL.Path == "/check_adres_validation3.php":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			validatedDomain = r.Form.Get("dmn")
			if validatedDomain != "dynamic-mail.com" {
				t.Fatalf("validated domain=%q, want dynamic-mail.com", validatedDomain)
			}
			_, _ = w.Write([]byte(`{"status":"good"}`))
		case strings.Contains(r.URL.Path, "kirodyn@dynamic-mail.com"):
			_, _ = w.Write([]byte(`<span id="email_ch_text">kirodyn@dynamic-mail.com</span>`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewGeneratorEmailService("")
	service.baseURL = server.URL
	service.localGenerator = func() string { return "kirodyn" }
	service.domains = []string{"fallback-mail.com"}

	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError error: %v", err)
	}
	if address != "kirodyn@dynamic-mail.com" {
		t.Fatalf("address=%q, want dynamic domain", address)
	}
}
