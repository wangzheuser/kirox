package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStep5WorkflowInitBodyJSONUsesBrowserInsertionOrder(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasSuffix(req.URL.Path, "/api/execute") {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"workflowStateHandle":"next-handle"}`))
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.SigninBase = server.URL
	cfg.ViewBase = server.URL
	r := NewRegistrar(cfg)
	r.WorkflowHandle = "wf-handle"

	if err := r.Step5WorkflowInit(); err != nil {
		t.Fatalf("Step5WorkflowInit returned error: %v", err)
	}

	assertJSONOrder(t, rawBody, []string{`"stepId":`, `"workflowStateHandle":`, `"inputs":`, `"requestId":`})
	assertContains(t, rawBody, `{"input_type":"FingerPrintRequestInput","fingerPrint":`)
}

func TestStep6SubmitEmailBodyJSONUsesBrowserInsertionOrder(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody = string(body)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"workflowStateHandle":"next-handle","redirect":{"url":"https://example.test/signup?workflowStateHandle=next-handle"}}`))
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.SigninBase = server.URL
	cfg.ViewBase = server.URL
	r := NewRegistrar(cfg)
	r.Email = "new@example.test"
	r.WorkflowHandle = "wf-handle"
	r.VisitorID = "visitor-id"

	status, err := r.Step6SubmitEmail()
	if err != nil {
		t.Fatalf("Step6SubmitEmail returned error: %v", err)
	}
	if status != "signup" {
		t.Fatalf("status=%q, want signup", status)
	}

	assertJSONOrder(t, rawBody, []string{`"stepId":`, `"workflowStateHandle":`, `"actionId":`, `"inputs":`, `"visitorId":`, `"requestId":`})
	assertJSONOrder(t, rawBody, []string{`"UserRequestInput"`, `"ApplicationTypeRequestInput"`, `"UserEventRequestInput"`, `"FingerPrintRequestInput"`})
	assertContains(t, rawBody, `{"input_type":"ApplicationTypeRequestInput","applicationType":"SSO_INDIVIDUAL_ID"}`)
	assertContains(t, rawBody, `{"input_type":"UserEventRequestInput","directoryId":"`+cfg.DirectoryID+`","userName":"new@example.test","userEvents":[{"input_type":"UserEvent","eventType":"PAGE_SUBMIT","pageName":"IDENTIFICATION","timeSpentOnPage":5000}]}`)
	assertContains(t, rawBody, `{"input_type":"FingerPrintRequestInput","fingerPrint":`)
}

func TestStep7SignupBodyJSONUsesBrowserInsertionOrder(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBody = string(body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"redirect":{"url":"https://example.test/signup?workflowStateHandle=signup-next"}}`))
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.SigninBase = server.URL
	cfg.ViewBase = server.URL
	r := NewRegistrar(cfg)
	r.Email = "new@example.test"
	r.WorkflowHandle = "wf-handle"
	r.VisitorID = "visitor-id"

	if err := r.Step7Signup(); err != nil {
		t.Fatalf("Step7Signup returned error: %v", err)
	}

	assertJSONOrder(t, rawBody, []string{`"stepId":`, `"workflowStateHandle":`, `"actionId":`, `"inputs":`, `"visitorId":`, `"requestId":`})
	assertJSONOrder(t, rawBody, []string{`"UserRequestInput"`, `"FingerPrintRequestInput"`})
	assertContains(t, rawBody, `{"input_type":"UserRequestInput","username":"new@example.test"}`)
	assertContains(t, rawBody, `{"input_type":"FingerPrintRequestInput","fingerPrint":`)
}

func TestStep7_5SignupInitBodiesJSONUseBrowserInsertionOrder(t *testing.T) {
	var rawBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !strings.HasSuffix(req.URL.Path, "/signup/api/execute") {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		rawBodies = append(rawBodies, string(body))
		w.WriteHeader(http.StatusOK)
		if len(rawBodies) == 1 {
			_, _ = w.Write([]byte(`{"workflowStateHandle":"signup-next","stepId":"start"}`))
			return
		}
		_, _ = w.Write([]byte(`{"workflowStateHandle":"signup-done","redirect":{"url":"https://profile.example.test/?workflowID=workflow-id#/signup/start"}}`))
	}))
	defer server.Close()

	cfg := NewConfig()
	cfg.SigninBase = server.URL
	cfg.ViewBase = server.URL
	r := NewRegistrar(cfg)
	r.Email = "new@example.test"
	r.WorkflowHandle = "wf-handle"
	r.VisitorID = "visitor-id"

	if err := r.Step7_5SignupInit(); err != nil {
		t.Fatalf("Step7_5SignupInit returned error: %v", err)
	}
	if len(rawBodies) != 2 {
		t.Fatalf("captured %d requests, want 2", len(rawBodies))
	}
	for i, rawBody := range rawBodies {
		assertJSONOrder(t, rawBody, []string{`"stepId":`, `"workflowStateHandle":`, `"inputs":`, `"visitorId":`, `"requestId":`})
		assertJSONOrder(t, rawBody, []string{`"UserRequestInput"`, `"FingerPrintRequestInput"`})
		assertContains(t, rawBody, `{"input_type":"UserRequestInput","username":"new@example.test"}`)
		assertContains(t, rawBody, `{"input_type":"FingerPrintRequestInput","fingerPrint":`)
		if i == 0 && !strings.Contains(rawBody, `"stepId":""`) {
			t.Fatalf("first signup init request should keep empty stepId: %s", rawBody)
		}
		if i == 1 && !strings.Contains(rawBody, `"stepId":"start"`) {
			t.Fatalf("second signup init request should use start stepId: %s", rawBody)
		}
	}
}

func assertContains(t *testing.T, raw, want string) {
	t.Helper()
	if !strings.Contains(raw, want) {
		t.Fatalf("JSON missing %s: %s", want, raw)
	}
}
