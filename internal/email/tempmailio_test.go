package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTempMailIOCreateUsesPreferredDomain(t *testing.T) {
	var posted map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/email/new" {
			t.Fatalf("path=%s, want /email/new", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"email":"codexname@bltiwd.com","token":"tok"}`))
	}))
	defer server.Close()

	service := NewTempMailIOService("", "bltiwd.com")
	service.baseURL = server.URL
	service.nameGenerator = func() string { return "codexname" }

	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError error: %v", err)
	}
	if address != "codexname@bltiwd.com" || service.GetAddress() != address {
		t.Fatalf("address=%q stored=%q", address, service.GetAddress())
	}
	if posted["name"] != "codexname" || posted["domain"] != "bltiwd.com" {
		t.Fatalf("posted=%v, want name/domain", posted)
	}
}

func TestTempMailIOWaitForCodeReadsMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/email/codex@bltiwd.com/messages") {
			t.Fatalf("path=%s, want messages path", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"m1","from":"no-reply@signin.aws","subject":"Verify your AWS Builder ID email address","body_text":"Your verification code is 246810"}]`))
	}))
	defer server.Close()

	service := NewTempMailIOService("", "bltiwd.com")
	service.baseURL = server.URL
	service.address = "codex@bltiwd.com"

	code, err := service.WaitForCode(2, 1)
	if err != nil {
		t.Fatalf("WaitForCode error: %v", err)
	}
	if code != "246810" {
		t.Fatalf("code=%q, want 246810", code)
	}
}
