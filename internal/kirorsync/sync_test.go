package kirorsync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSyncAccountsTreatsExistingCredentialAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/admin/credentials" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"type":"invalid_request","message":"凭据无效：凭据已存在（refreshToken 重复）"}}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "exists@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Error != "" {
		t.Fatalf("unexpected sync error: %s", result.Error)
	}
	if result.Total != 1 || result.Success != 1 || result.Failed != 0 {
		t.Fatalf("existing credential should count as success, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || !result.Details[0].Success {
		t.Fatalf("detail should be success for existing credential: %#v", result.Details)
	}
	if !strings.Contains(result.Details[0].Error, "已存在") {
		t.Fatalf("detail should retain existing-credential hint: %#v", result.Details[0])
	}
}

func TestSyncAccountsKeepsOtherBadRequestAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"type":"invalid_request","message":"refreshToken 格式错误"}}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "bad@example.com", "refreshToken": "bad", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("bad request should remain failure, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
}

func TestSyncAccountsCountsOrdinarySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body addCredentialRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Email != "ok@example.com" {
			t.Fatalf("expected request email ok@example.com, got %q", body.Email)
		}
		fmt.Fprint(w, `{"success":true,"credentialId":123,"email":"ok@example.com"}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "ok@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 1 || result.Failed != 0 {
		t.Fatalf("ordinary success should count as success, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
}

func TestSyncAccountsRetriesNetworkErrorOnceAndCountsSuccess(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("test server response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack failed: %v", err)
			}
			conn.Close()
			return
		}
		fmt.Fprint(w, `{"success":true,"credentialId":456,"email":"retry@example.com"}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "retry@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if attempts != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", attempts)
	}
	if result.Total != 1 || result.Success != 1 || result.Failed != 0 {
		t.Fatalf("network retry success should count as success, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || !result.Details[0].Success || result.Details[0].CredentialID != 456 {
		t.Fatalf("detail should reflect retry success: %#v", result.Details)
	}
}

func TestSyncAccountsRetriesRetryableErrorImmediatelyBeforeNextAccount(t *testing.T) {
	var seen []string
	var seenEmails []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body addCredentialRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		seen = append(seen, body.RefreshToken)
		seenEmails = append(seenEmails, body.Email)
		if body.RefreshToken == "first" && len(seen) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `temporary server error`)
			return
		}
		fmt.Fprintf(w, `{"success":true,"credentialId":%d}`, len(seen))
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "first@example.com", "refreshToken": "first", "clientId": "client", "clientSecret": "secret"},
		{"email": "second@example.com", "refreshToken": "second", "clientId": "client", "clientSecret": "secret"},
	})

	wantOrder := []string{"first", "first", "second"}
	if strings.Join(seen, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("retry should happen immediately before next account, got order %#v", seen)
	}
	wantEmails := []string{"first@example.com", "first@example.com", "second@example.com"}
	if strings.Join(seenEmails, ",") != strings.Join(wantEmails, ",") {
		t.Fatalf("each request should include its account email, got emails %#v", seenEmails)
	}
	if result.Total != 2 || result.Success != 2 || result.Failed != 0 {
		t.Fatalf("retryable server error should recover immediately, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
}

func TestSyncAccountsTrimsEmailInCredentialRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body addCredentialRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body.Email != "trim@example.com" {
			t.Fatalf("expected trimmed request email trim@example.com, got %q", body.Email)
		}
		fmt.Fprint(w, `{"success":true,"credentialId":789,"email":"trim@example.com"}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": " trim@example.com ", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 1 || result.Failed != 0 {
		t.Fatalf("trimmed email success should count as success, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
}

func TestSyncAccountsRetriesNetworkErrorOnceAndCountsFailure(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("test server response writer does not support hijacking")
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("hijack failed: %v", err)
		}
		conn.Close()
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "fail-retry@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if attempts != 2 {
		t.Fatalf("expected exactly 2 attempts, got %d", attempts)
	}
	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("network retry failure should count as final failure, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
}

func TestSyncAccountsDoesNotRetryNonRetryableBadRequest(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"type":"invalid_request","message":"refreshToken 格式错误"}}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "bad-no-retry@example.com", "refreshToken": "bad", "clientId": "client", "clientSecret": "secret"},
	})

	if attempts != 1 {
		t.Fatalf("expected exactly 1 attempt for non-retryable bad request, got %d", attempts)
	}
	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("bad request should remain failure, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
}
