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
	if result.Details[0].Rejected {
		t.Fatalf("existing credential must not be rejected for local deletion: %#v", result.Details[0])
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

func TestSyncAccountsRejectsModelForbiddenWrappedByBadGateway(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `{"error":{"type":"api_error","message":"上游服务错误: ListAvailableModels 请求失败: HTTP 403 Forbidden body={}"}}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "forbidden@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if attempts != 1 {
		t.Fatalf("permanent model forbidden should not be retried, got attempts=%d", attempts)
	}
	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("model forbidden should fail sync, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || !result.Details[0].Rejected || result.Details[0].RejectReason == "" {
		t.Fatalf("model forbidden should be rejected for local deletion: %#v", result.Details)
	}
}

func TestSyncAccountsKeepsOrdinaryBadGatewayAsTransient(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprint(w, `temporary upstream gateway error`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "transient-502@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if attempts != 2 {
		t.Fatalf("ordinary 502 should still retry once, got attempts=%d", attempts)
	}
	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("ordinary 502 should fail after retry, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || result.Details[0].Rejected {
		t.Fatalf("ordinary 502 must not reject local account: %#v", result.Details)
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
		fmt.Fprint(w, `{"success":true,"credentialId":123,"email":"ok@example.com","balance":{"balance":1}}`)
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
		fmt.Fprint(w, `{"success":true,"credentialId":456,"email":"retry@example.com","balance":{"balance":1}}`)
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
		fmt.Fprintf(w, `{"success":true,"credentialId":%d,"balance":{"balance":1}}`, len(seen))
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
		fmt.Fprint(w, `{"success":true,"credentialId":789,"email":"trim@example.com","balance":{"balance":1}}`)
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

func TestSyncAccountsRejectsExpiredTokenFromAddCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"type":"invalid_request","message":"凭据无效: OAuth/IdC 凭证已过期或无效，需要重新认证: ExpiredToken"}}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "expired@example.com", "refreshToken": "expired", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("expired token should fail, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || !result.Details[0].Rejected || result.Details[0].RejectReason == "" {
		t.Fatalf("expired token should be rejected for local deletion: %#v", result.Details)
	}
}

func TestSyncAccountsRejectsBannedCredentialFromForcedBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/credentials":
			fmt.Fprint(w, `{"success":true,"credentialId":42,"email":"banned@example.com"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/credentials/42/balance":
			if r.URL.Query().Get("force_refresh") != "true" {
				t.Fatalf("force_refresh should be true, got query %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":{"type":"invalid_request","message":"获取余额失败: 凭证已被封禁或禁用: TEMPORARILY_SUSPENDED"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "banned@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("banned balance should fail, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || !result.Details[0].Rejected || !strings.Contains(result.Details[0].RejectReason, "封禁") {
		t.Fatalf("banned balance should mark rejected: %#v", result.Details)
	}
}

func TestSyncAccountsKeepsCredentialWhenForcedBalanceIsTransient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/credentials":
			fmt.Fprint(w, `{"success":true,"credentialId":77,"email":"slow@example.com"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/credentials/77/balance":
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"error":{"type":"api_error","message":"rate limited"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "slow@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 1 || result.Failed != 0 {
		t.Fatalf("transient balance error should keep sync success, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || result.Details[0].Rejected || result.Details[0].Verified || result.Details[0].VerificationError == "" {
		t.Fatalf("transient balance error should be unverified but not rejected: %#v", result.Details)
	}
}

func TestSyncAccountsKeepsCredentialWhenForcedBalanceReturnsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/admin/credentials":
			fmt.Fprint(w, `{"success":true,"credentialId":78,"email":"server-error@example.com"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/admin/credentials/78/balance":
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"error":{"type":"api_error","message":"upstream unavailable"}}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "server-error@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 1 || result.Failed != 0 {
		t.Fatalf("5xx balance error should keep sync success, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || result.Details[0].Rejected || result.Details[0].Verified || result.Details[0].VerificationError == "" {
		t.Fatalf("5xx balance error should be unverified but not rejected: %#v", result.Details)
	}
}

func TestSyncAccountsDoesNotRejectAdminAPIKeyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"type":"authentication_error","message":"Invalid or missing admin API key"}}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "bad-admin-key", []map[string]interface{}{
		{"email": "admin-error@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("admin key error should fail without deletion, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || result.Details[0].Rejected {
		t.Fatalf("admin key error must not reject local account: %#v", result.Details)
	}
}

func TestSyncAccountsDoesNotRejectDisabledManagementState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"type":"invalid_request","message":"凭据 disabled=true disabledReason=Manual"}}`)
	}))
	defer server.Close()

	result := SyncAccounts(server.URL, "test-key", []map[string]interface{}{
		{"email": "disabled@example.com", "refreshToken": "refresh", "clientId": "client", "clientSecret": "secret"},
	})

	if result.Total != 1 || result.Success != 0 || result.Failed != 1 {
		t.Fatalf("disabled management state should fail without deletion, got total=%d success=%d failed=%d details=%#v", result.Total, result.Success, result.Failed, result.Details)
	}
	if len(result.Details) != 1 || result.Details[0].Rejected {
		t.Fatalf("disabled management state must not reject local account: %#v", result.Details)
	}
}
