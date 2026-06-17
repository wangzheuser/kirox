package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStep6SubmitEmailBlocked400ReturnsErrorInsteadOfSignup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/execute") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errorCode":"BLOCKED","message":"Request was blocked by TES."}`))
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.SigninBase = server.URL
	cfg.ViewBase = server.URL
	reg := NewRegistrar(cfg)
	reg.Email = "blocked@example.test"
	reg.WorkflowHandle = "wf-handle"

	status, err := reg.Step6SubmitEmail()
	if err == nil {
		t.Fatalf("Step6SubmitEmail should reject TES/BLOCKED 400 instead of returning status %q", status)
	}
	if status != "" {
		t.Fatalf("blocked Step6 should not return a next status, got %q", status)
	}
	if !strings.Contains(err.Error(), "BLOCKED") || !strings.Contains(err.Error(), "提交邮箱失败") {
		t.Fatalf("blocked Step6 error should preserve evidence, got %v", err)
	}
}

func TestStep6SubmitEmailSignup400RequiresSignupRedirectEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"workflowStateHandle": "next-handle",
			"redirect":            map[string]interface{}{"url": "https://example.test/signup?workflowStateHandle=next-handle"},
		})
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.SigninBase = server.URL
	cfg.ViewBase = server.URL
	reg := NewRegistrar(cfg)
	reg.Email = "new@example.test"
	reg.WorkflowHandle = "wf-handle"

	status, err := reg.Step6SubmitEmail()
	if err != nil {
		t.Fatalf("Step6SubmitEmail signup 400 returned error: %v", err)
	}
	if status != "signup" {
		t.Fatalf("Step6SubmitEmail status=%q, want signup", status)
	}
	if reg.WorkflowHandle != "next-handle" {
		t.Fatalf("workflow handle=%q, want next-handle", reg.WorkflowHandle)
	}
}
