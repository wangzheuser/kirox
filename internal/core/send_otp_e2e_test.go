package core

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStep8ProfileStartBodyJSONUsesBrowserInsertionOrder(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/start" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"workflowState":"wf-state-from-server"}`))
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.ProfileBase = server.URL
	r := NewRegistrar(cfg)
	r.WorkflowID = "wf-id"
	r.Ubid = "ubid"
	r.VisitorID = "visitor-id"

	if err := r.Step8ProfileStart(); err != nil {
		t.Fatalf("Step8ProfileStart returned error: %v", err)
	}
	if r.WorkflowState != "wf-state-from-server" {
		t.Fatalf("WorkflowState=%q", r.WorkflowState)
	}

	assertJSONOrder(t, rawBody, []string{`"workflowID":`, `"browserData":`})
	assertJSONOrder(t, rawBody, []string{`"attributes":`, `"cookies":`})
	assertJSONOrder(t, rawBody, []string{`"fingerprint":`, `"eventTimestamp":`, `"timeSpentOnPage":`, `"eventType":`, `"ubid":`, `"visitorId":`})
	if !strings.Contains(rawBody, `"eventType":"PageLoad"`) {
		t.Fatalf("profile start body missing PageLoad event metadata: %s", rawBody)
	}
}

func TestStep9SendOTPLocalTargetAcceptsValidRequest(t *testing.T) {
	var seenEmail string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/send-otp" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		seenEmail, _ = payload["email"].(string)
		if payload["workflowState"] != "wf-state" {
			t.Fatalf("workflowState=%v", payload["workflowState"])
		}
		browserData, _ := payload["browserData"].(map[string]interface{})
		attrs, _ := browserData["attributes"].(map[string]interface{})
		if attrs["timeSpentOnPage"] != "0" || attrs["pageName"] != "EMAIL_COLLECTION" || attrs["eventType"] != "PageSubmit" {
			t.Fatalf("unexpected browser attributes: %#v", attrs)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.ProfileBase = server.URL
	r := NewRegistrar(cfg)
	r.WorkflowID = "wf-id"
	r.WorkflowState = "wf-state"
	r.Email = "user@example.test"
	r.Ubid = "ubid"

	started := time.Now()
	if err := r.Step9SendOTP(); err != nil {
		t.Fatalf("Step9SendOTP returned error: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Step9SendOTP should not wait before send-otp, elapsed=%s", elapsed)
	}
	if seenEmail != r.Email {
		t.Fatalf("server saw email %q, want %q", seenEmail, r.Email)
	}
}

func TestStep9SendOTPBodyJSONUsesBrowserInsertionOrder(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/send-otp" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.ProfileBase = server.URL
	r := NewRegistrar(cfg)
	r.WorkflowID = "wf-id"
	r.WorkflowState = "wf-state"
	r.Email = "user@example.test"
	r.Ubid = "ubid"
	r.VisitorID = "visitor-id"

	if err := r.Step9SendOTP(); err != nil {
		t.Fatalf("Step9SendOTP returned error: %v", err)
	}

	assertJSONOrder(t, rawBody, []string{`"workflowState":`, `"email":`, `"browserData":`})
	assertJSONOrder(t, rawBody, []string{`"attributes":`, `"cookies":`})
	assertJSONOrder(t, rawBody, []string{`"fingerprint":`, `"eventTimestamp":`, `"timeSpentOnPage":`, `"pageName":`, `"eventType":`, `"ubid":`, `"visitorId":`})
	if !strings.Contains(rawBody, `"pageName":"EMAIL_COLLECTION"`) || !strings.Contains(rawBody, `"eventType":"PageSubmit"`) {
		t.Fatalf("send-otp body missing expected event metadata: %s", rawBody)
	}
}

func assertJSONOrder(t *testing.T, raw string, orderedKeys []string) {
	t.Helper()
	last := -1
	for _, key := range orderedKeys {
		idx := strings.Index(raw, key)
		if idx < 0 {
			t.Fatalf("%s missing from JSON: %s", key, raw)
		}
		if idx <= last {
			t.Fatalf("%s appears out of order in JSON: %s", key, raw)
		}
		last = idx
	}
}
