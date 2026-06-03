package kirorsync

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
