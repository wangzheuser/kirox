package data

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestAccounts(t *testing.T, dir string, accounts []map[string]interface{}) {
	t.Helper()
	b, err := json.Marshal(accounts)
	if err != nil {
		t.Fatalf("marshal accounts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), b, 0o600); err != nil {
		t.Fatalf("write accounts.json: %v", err)
	}
}

func accountByEmail(t *testing.T, accounts []map[string]interface{}, email string) map[string]interface{} {
	t.Helper()
	for _, acc := range accounts {
		if acc["email"] == email {
			return acc
		}
	}
	t.Fatalf("account %s not found in %#v", email, accounts)
	return nil
}

func TestImportAccountPoolJSONAddsAndUpdatesByEmail(t *testing.T) {
	dir := t.TempDir()
	writeTestAccounts(t, dir, []map[string]interface{}{
		{
			"email":        "existing@example.com",
			"password":     "old-password",
			"clientId":     "old-client",
			"clientSecret": "old-secret",
			"refreshToken": "old-refresh",
			"accessToken":  "old-access",
			"subscription": "KIRO FREE",
			"provider":     "CustomProvider",
			"region":       "eu-west-1",
			"time":         "2026-05-01 12:00:00",
		},
	})

	summary, err := ImportAccountPoolJSON(dir, `[
	  {
	    "email": "existing@example.com",
	    "password": "new-password",
	    "clientId": "new-client",
	    "clientSecret": "new-secret",
	    "accessToken": "new-access",
	    "refreshToken": "new-refresh",
	    "priority": 601
	  },
	  {
	    "email": "new@example.com",
	    "password": "new-pass",
	    "clientId": "new-client-id",
	    "clientSecret": "new-client-secret",
	    "accessToken": "new-access-token",
	    "refreshToken": "new-refresh-token"
	  }
	]`)
	if err != nil {
		t.Fatalf("ImportAccountPoolJSON returned error: %v", err)
	}
	if summary.Imported != 1 || summary.Updated != 1 || summary.Skipped != 0 || summary.Total != 2 {
		t.Fatalf("unexpected summary: %#v", summary)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	existing := accountByEmail(t, accounts, "existing@example.com")
	if existing["password"] != "new-password" ||
		existing["clientId"] != "new-client" ||
		existing["clientSecret"] != "new-secret" ||
		existing["accessToken"] != "new-access" ||
		existing["refreshToken"] != "new-refresh" ||
		existing["priority"] != float64(601) {
		t.Fatalf("existing account was not updated correctly: %#v", existing)
	}
	if existing["subscription"] != "KIRO FREE" || existing["provider"] != "CustomProvider" || existing["region"] != "eu-west-1" || existing["time"] != "2026-05-01 12:00:00" {
		t.Fatalf("existing account did not preserve local-only fields: %#v", existing)
	}

	added := accountByEmail(t, accounts, "new@example.com")
	if added["provider"] != "BuilderId" || added["region"] != "us-east-1" {
		t.Fatalf("new account defaults not set: %#v", added)
	}
	if added["time"] == "" {
		t.Fatalf("new account should receive time: %#v", added)
	}
	if added["priority"] != float64(9999) {
		t.Fatalf("new account priority=%#v, want 9999", added["priority"])
	}
}

func TestImportAccountPoolJSONReportsInvalidEmptyAndSkippedRows(t *testing.T) {
	dir := t.TempDir()

	if _, err := ImportAccountPoolJSON(dir, `{bad json`); err == nil {
		t.Fatal("invalid JSON should return an error")
	}

	emptySummary, err := ImportAccountPoolJSON(dir, `[]`)
	if err != nil {
		t.Fatalf("empty JSON array should not error: %v", err)
	}
	if emptySummary.Imported != 0 || emptySummary.Updated != 0 || emptySummary.Skipped != 0 || emptySummary.Total != 0 {
		t.Fatalf("unexpected empty summary: %#v", emptySummary)
	}

	skippedSummary, err := ImportAccountPoolJSON(dir, `[{"password":"missing-email"}]`)
	if err != nil {
		t.Fatalf("missing email row should not error: %v", err)
	}
	if skippedSummary.Imported != 0 || skippedSummary.Updated != 0 || skippedSummary.Skipped != 1 || skippedSummary.Total != 0 {
		t.Fatalf("unexpected skipped summary: %#v", skippedSummary)
	}
}

func TestExportAccountPoolJSONUsesReferenceShapeAndPriority(t *testing.T) {
	dir := t.TempDir()
	writeTestAccounts(t, dir, []map[string]interface{}{
		{
			"email":        "first@example.com",
			"password":     "pass",
			"clientId":     "client",
			"clientSecret": "secret",
			"accessToken":  "access",
			"refreshToken": "refresh",
			"time":         "2026-05-01 12:00:00",
			"subscription": "KIRO FREE",
		},
	})

	exported, count, err := ExportAccountPoolJSON(dir)
	if err != nil {
		t.Fatalf("ExportAccountPoolJSON returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(exported), &rows); err != nil {
		t.Fatalf("exported JSON should be valid: %v\n%s", err, exported)
	}
	row := rows[0]
	if row["email"] != "first@example.com" ||
		row["password"] != "pass" ||
		row["clientId"] != "client" ||
		row["clientSecret"] != "secret" ||
		row["accessToken"] != "access" ||
		row["refreshToken"] != "refresh" ||
		row["priority"] != float64(9999) {
		t.Fatalf("unexpected exported row: %#v", row)
	}
	if _, ok := row["subscription"]; ok {
		t.Fatalf("export should omit non-reference fields: %#v", row)
	}

	keys := []string{`"email"`, `"password"`, `"clientId"`, `"clientSecret"`, `"accessToken"`, `"refreshToken"`, `"priority"`}
	last := -1
	for _, key := range keys {
		idx := strings.Index(exported, key)
		if idx == -1 {
			t.Fatalf("exported JSON missing key %s:\n%s", key, exported)
		}
		if idx <= last {
			t.Fatalf("key %s is out of order in:\n%s", key, exported)
		}
		last = idx
	}
}

func TestListAccountPoolDefaultsMissingKiroRSSyncedToFalse(t *testing.T) {
	dir := t.TempDir()
	writeTestAccounts(t, dir, []map[string]interface{}{
		{"email": "legacy@example.com", "refreshToken": "refresh"},
	})

	accounts, err := ListAccountPool(dir)
	if err != nil {
		t.Fatalf("ListAccountPool: %v", err)
	}
	got := accountByEmail(t, accounts, "legacy@example.com")
	if synced, ok := got["kiroRsSynced"].(bool); !ok || synced {
		t.Fatalf("legacy account should display as kiroRsSynced=false: %#v", got)
	}
}

func TestImportAccountPoolJSONResetsKiroRSSyncedWhenCredentialChanges(t *testing.T) {
	dir := t.TempDir()
	writeTestAccounts(t, dir, []map[string]interface{}{
		{
			"email":        "existing@example.com",
			"refreshToken": "old-refresh",
			"kiroRsSynced": true,
		},
	})

	_, err := ImportAccountPoolJSON(dir, `[{"email":"existing@example.com","refreshToken":"new-refresh"}]`)
	if err != nil {
		t.Fatalf("ImportAccountPoolJSON: %v", err)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	got := accountByEmail(t, accounts, "existing@example.com")
	if got["refreshToken"] != "new-refresh" {
		t.Fatalf("refreshToken was not updated: %#v", got)
	}
	if synced, ok := got["kiroRsSynced"].(bool); !ok || synced {
		t.Fatalf("changed credential should reset kiroRsSynced=false: %#v", got)
	}
}

func TestMarkKiroRSSyncedMarksOnlySuccessfulEmails(t *testing.T) {
	dir := t.TempDir()
	writeTestAccounts(t, dir, []map[string]interface{}{
		{"email": "first@example.com", "kiroRsSynced": false},
		{"email": "second@example.com", "kiroRsSynced": false},
	})

	updated, err := MarkKiroRSSynced(dir, []string{"second@example.com", "missing@example.com"})
	if err != nil {
		t.Fatalf("MarkKiroRSSynced: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated=%d, want 1", updated)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	first := accountByEmail(t, accounts, "first@example.com")
	second := accountByEmail(t, accounts, "second@example.com")
	if synced, _ := first["kiroRsSynced"].(bool); synced {
		t.Fatalf("unsuccessful account should remain unsynced: %#v", first)
	}
	if synced, ok := second["kiroRsSynced"].(bool); !ok || !synced {
		t.Fatalf("successful account should be marked synced: %#v", second)
	}
}

func TestImportAccountPoolJSONPreservesExplicitPriority(t *testing.T) {
	dir := t.TempDir()

	_, err := ImportAccountPoolJSON(dir, `[{
	  "email": "explicit@example.com",
	  "password": "pass",
	  "priority": 601
	}]`)
	if err != nil {
		t.Fatalf("ImportAccountPoolJSON returned error: %v", err)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	got := accountByEmail(t, accounts, "explicit@example.com")
	if got["priority"] != float64(601) {
		t.Fatalf("explicit priority should be preserved: %#v", got)
	}
}

func TestDeleteAccountsRemovesEmailsCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	writeTestAccounts(t, dir, []map[string]interface{}{
		{"email": "First@example.com"},
		{"email": "second@example.com"},
		{"email": "keep@example.com"},
	})

	removed, err := DeleteAccounts(dir, []string{"first@EXAMPLE.com", "SECOND@example.com"})
	if err != nil {
		t.Fatalf("DeleteAccounts: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed=%d, want 2", removed)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0]["email"] != "keep@example.com" {
		t.Fatalf("unexpected remaining accounts: %#v", accounts)
	}
}

func TestDeleteAccountsMissingEmailsDoNotAffectOtherRecords(t *testing.T) {
	dir := t.TempDir()
	writeTestAccounts(t, dir, []map[string]interface{}{
		{"email": "first@example.com"},
		{"email": "second@example.com"},
	})

	removed, err := DeleteAccounts(dir, []string{"missing@example.com"})
	if err != nil {
		t.Fatalf("DeleteAccounts: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed=%d, want 0", removed)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("missing delete should not change accounts: %#v", accounts)
	}
}
