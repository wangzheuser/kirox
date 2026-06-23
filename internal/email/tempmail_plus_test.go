package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withTempMailPlusTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldBase := tempMailPlusBaseURL
	tempMailPlusBaseURL = server.URL
	t.Cleanup(func() { tempMailPlusBaseURL = oldBase })
}

func TestTempMailPlusCreateUsesSupportedDomain(t *testing.T) {
	oldDomains := tempMailPlusDomains
	oldIx := tempMailPlusDomainIx
	tempMailPlusDomains = []string{"fexpost.com"}
	tempMailPlusDomainIx = 0
	t.Cleanup(func() {
		tempMailPlusDomains = oldDomains
		tempMailPlusDomainIx = oldIx
	})

	withTempMailPlusTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body>No dynamic domains in this fixture</body></html>`))
			return
		case "/api/mails":
			if !strings.Contains(r.URL.Query().Get("email"), "@fexpost.com") {
				t.Fatalf("email=%q, want fexpost.com", r.URL.Query().Get("email"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": true, "mail_list": []interface{}{}})
			return
		default:
			t.Fatalf("path=%q, want /api/mails", r.URL.Path)
		}
	})

	service := NewTempMailPlusService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if !strings.HasSuffix(address, "@fexpost.com") {
		t.Fatalf("address=%q, want fexpost.com", address)
	}
}

func TestTempMailPlusCreateFallsBackToNextDomainWhenFirstDomainFails(t *testing.T) {
	oldDomains := tempMailPlusDomains
	oldIx := tempMailPlusDomainIx
	tempMailPlusDomains = []string{"fexpost.com", "fextemp.com"}
	tempMailPlusDomainIx = 0
	t.Cleanup(func() {
		tempMailPlusDomains = oldDomains
		tempMailPlusDomainIx = oldIx
	})

	withTempMailPlusTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<html><body>No dynamic domains in this fixture</body></html>`))
			return
		}
		if r.URL.Path != "/api/mails" {
			t.Fatalf("path=%q, want /api/mails", r.URL.Path)
		}
		email := r.URL.Query().Get("email")
		if strings.HasSuffix(email, "@fexpost.com") {
			http.Error(w, "blocked", http.StatusBadRequest)
			return
		}
		if !strings.HasSuffix(email, "@fextemp.com") {
			t.Fatalf("email=%q, want fallback fextemp.com", email)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": true, "mail_list": []interface{}{}})
	})

	service := NewTempMailPlusService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if !strings.HasSuffix(address, "@fextemp.com") {
		t.Fatalf("address=%q, want fallback fextemp.com", address)
	}
}

func TestTempMailPlusCreateUsesHomepageDomainsBeforeFallback(t *testing.T) {
	oldDomains := tempMailPlusDomains
	oldIx := tempMailPlusDomainIx
	tempMailPlusDomains = []string{"fallback-plus.com"}
	tempMailPlusDomainIx = 0
	t.Cleanup(func() {
		tempMailPlusDomains = oldDomains
		tempMailPlusDomainIx = oldIx
	})

	withTempMailPlusTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<button type="button" class="dropdown-item">dynamic-plus.com</button>`))
		case "/api/mails":
			email := r.URL.Query().Get("email")
			if !strings.HasSuffix(email, "@dynamic-plus.com") {
				t.Fatalf("email=%q, want dynamic-plus.com", email)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"result": true, "mail_list": []interface{}{}})
		default:
			t.Fatalf("unexpected path=%q", r.URL.Path)
		}
	})

	service := NewTempMailPlusService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if !strings.HasSuffix(address, "@dynamic-plus.com") {
		t.Fatalf("address=%q, want dynamic-plus.com", address)
	}
}

func TestTempMailPlusWaitForCodeReadsMessageDetail(t *testing.T) {
	withTempMailPlusTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/mails":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result": true,
				"mail_list": []map[string]interface{}{
					{"mail_id": "abc", "from": "no-reply@signin.aws", "subject": "Verify your AWS Builder ID email address"},
				},
			})
		case "/api/mails/abc":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"result":  true,
				"subject": "Verify your AWS Builder ID email address",
				"html":    "<div>Verification code: <b>112233</b></div>",
			})
		default:
			t.Fatalf("unexpected path=%q", r.URL.Path)
		}
	})

	service := NewTempMailPlusService("")
	service.address = "codextest@fexpost.com"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "112233" {
		t.Fatalf("code=%q, want 112233", code)
	}
}
