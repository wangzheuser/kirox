package task

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"reg_go/internal/data"
	"reg_go/internal/kirorsync"
)

func TestProcessKiroRSAutoSyncJobMarksAccountSynced(t *testing.T) {
	dir := t.TempDir()
	account := map[string]interface{}{
		"email":        "synced@example.com",
		"refreshToken": "refresh-token",
		"clientId":     "client-id",
		"kiroRsSynced": false,
	}
	writeAccountsJSONForAutoSyncTest(t, dir, []map[string]interface{}{account})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/admin/credentials" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"success":true,"credentialId":1,"email":"synced@example.com","balance":{"ok":true}}`)
	}))
	defer server.Close()

	events := captureKiroRSSyncEvents(t)
	processKiroRSAutoSyncJob(kiroRSAutoSyncJob{
		outDir:  dir,
		apiURL:  server.URL,
		apiKey:  "test-key",
		account: cloneKiroRSSyncAccount(account),
	})

	accounts, err := data.LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one account, got %#v", accounts)
	}
	if synced, _ := accounts[0]["kiroRsSynced"].(bool); !synced {
		t.Fatalf("auto sync should mark account synced: %#v", accounts[0])
	}
	result := mustReceiveKiroRSSyncEvent(t, events)
	if result.Success != 1 || result.Failed != 0 {
		t.Fatalf("unexpected sync event: %#v", result)
	}
}

func TestProcessKiroRSAutoSyncJobDeletesRejectedAccount(t *testing.T) {
	dir := t.TempDir()
	account := map[string]interface{}{
		"email":        "rejected@example.com",
		"refreshToken": "refresh-token",
		"clientId":     "client-id",
		"kiroRsSynced": false,
	}
	writeAccountsJSONForAutoSyncTest(t, dir, []map[string]interface{}{account})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/admin/credentials" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"message":"ListAvailableModels 请求失败: HTTP 403 Forbidden"}}`)
	}))
	defer server.Close()

	events := captureKiroRSSyncEvents(t)
	processKiroRSAutoSyncJob(kiroRSAutoSyncJob{
		outDir:  dir,
		apiURL:  server.URL,
		apiKey:  "test-key",
		account: cloneKiroRSSyncAccount(account),
	})

	accounts, err := data.LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("rejected account should be deleted, got %#v", accounts)
	}
	result := mustReceiveKiroRSSyncEvent(t, events)
	if result.Success != 0 || result.Failed != 1 || len(result.Details) != 1 || !result.Details[0].Rejected {
		t.Fatalf("unexpected rejected sync event: %#v", result)
	}
}

func captureKiroRSSyncEvents(t *testing.T) <-chan kirorsync.SyncResult {
	t.Helper()
	events := make(chan kirorsync.SyncResult, 1)
	oldCallback := Manager.OnSyncResult
	Manager.OnSyncResult = func(result interface{}) {
		if syncResult, ok := result.(kirorsync.SyncResult); ok {
			events <- syncResult
		}
	}
	t.Cleanup(func() {
		Manager.OnSyncResult = oldCallback
	})
	return events
}

func mustReceiveKiroRSSyncEvent(t *testing.T, events <-chan kirorsync.SyncResult) kirorsync.SyncResult {
	t.Helper()
	select {
	case result := <-events:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for kiro.rs sync event")
		return kirorsync.SyncResult{}
	}
}

func writeAccountsJSONForAutoSyncTest(t *testing.T, dir string, accounts []map[string]interface{}) {
	t.Helper()
	for _, account := range accounts {
		account["status"] = "success"
		account["aws_token"] = map[string]interface{}{"refreshToken": account["refreshToken"]}
		account["client_id"] = account["clientId"]
		if err := data.SaveKiroSuccess(account, dir); err != nil {
			t.Fatalf("SaveKiroSuccess: %v", err)
		}
	}
}
