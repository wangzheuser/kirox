package main

import "testing"

func TestSelectKiroRSAccountsForModeUnsyncedFiltersSyncedAccounts(t *testing.T) {
	accounts := []map[string]interface{}{
		{"email": "synced@example.com", "kiroRsSynced": true},
		{"email": "unsynced@example.com", "kiroRsSynced": false},
		{"email": "legacy@example.com"},
	}

	selected, errMsg := selectKiroRSAccountsForMode(accounts, "unsynced")
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if len(selected) != 2 || selected[0]["email"] != "unsynced@example.com" || selected[1]["email"] != "legacy@example.com" {
		t.Fatalf("unsynced mode should include false and missing statuses only: %#v", selected)
	}
}

func TestSelectKiroRSAccountsForModeAllIncludesEveryAccount(t *testing.T) {
	accounts := []map[string]interface{}{
		{"email": "synced@example.com", "kiroRsSynced": true},
		{"email": "unsynced@example.com", "kiroRsSynced": false},
	}

	selected, errMsg := selectKiroRSAccountsForMode(accounts, "all")
	if errMsg != "" {
		t.Fatalf("unexpected error: %s", errMsg)
	}
	if len(selected) != 2 {
		t.Fatalf("all mode should include every account: %#v", selected)
	}
}

func TestSelectKiroRSAccountsForModeRejectsInvalidMode(t *testing.T) {
	if _, errMsg := selectKiroRSAccountsForMode(nil, "bad"); errMsg == "" {
		t.Fatal("invalid mode should return an error message")
	}
}
