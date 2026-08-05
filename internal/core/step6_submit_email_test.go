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

func TestStep6SubmitEmailSignup400AcceptsCompatibleWorkflowHandle(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]interface{}
		wantSignup bool
	}{
		{
			name: "valid signup redirect",
			body: map[string]interface{}{
				"workflowStateHandle": "next-handle",
				"redirect":            map[string]interface{}{"url": "https://example.test/signup?workflowStateHandle=next-handle"},
			},
			wantSignup: true,
		},
		{
			name: "new identity handle without redirect",
			body: map[string]interface{}{
				"workflowStateHandle": "next-handle",
				"message":             map[string]interface{}{"errorCode": "ENTITY_DOES_NOT_EXIST"},
			},
			wantSignup: true,
		},
		{name: "arbitrary error with handle", body: map[string]interface{}{"workflowStateHandle": "next-handle", "message": map[string]interface{}{"errorCode": "UNEXPECTED_ERROR"}}},
		{name: "missing redirect and handle", body: map[string]interface{}{}},
		{name: "redirect missing handle", body: map[string]interface{}{"redirect": map[string]interface{}{"url": "https://example.test/signup"}}},
		{name: "non signup redirect", body: map[string]interface{}{"redirect": map[string]interface{}{"url": "https://example.test/login?workflowStateHandle=next-handle"}}},
		{
			name: "mismatched handles",
			body: map[string]interface{}{
				"workflowStateHandle": "other-handle",
				"redirect":            map[string]interface{}{"url": "https://example.test/signup?workflowStateHandle=next-handle"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(tt.body)
			}))
			defer server.Close()

			cfg := NewConfig()
			cfg.SigninBase = server.URL
			cfg.ViewBase = server.URL
			reg := NewRegistrar(cfg)
			reg.Email = "new@example.test"
			reg.WorkflowHandle = "wf-handle"

			status, err := reg.Step6SubmitEmail()
			if tt.wantSignup {
				if err != nil || status != "signup" || reg.WorkflowHandle != "next-handle" {
					t.Fatalf("valid signup response = (%q, %v, %q)", status, err, reg.WorkflowHandle)
				}
				return
			}
			if err == nil || status != "" || reg.WorkflowHandle != "wf-handle" {
				t.Fatalf("invalid signup response = (%q, %v, %q), want error and unchanged handle", status, err, reg.WorkflowHandle)
			}
		})
	}
}
