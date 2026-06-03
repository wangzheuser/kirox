package task

import (
	"testing"

	"reg_go/internal/kirorsync"
)

func TestFilterAccountsByEmailReturnsOnlyCurrentBatch(t *testing.T) {
	accounts := []map[string]interface{}{
		{"email": "old@example.com"},
		{"email": "new@example.com"},
		{"email": "another@example.com"},
	}

	got := filterAccountsByEmail(accounts, []string{"new@example.com"})

	if len(got) != 1 || got[0]["email"] != "new@example.com" {
		t.Fatalf("expected only current batch account, got %#v", got)
	}
}

func TestSuccessfulSyncEmailsReturnsOnlySuccessfulDetails(t *testing.T) {
	got := successfulSyncEmails(kirorsync.SyncResult{Details: []kirorsync.SyncDetail{
		{Email: "ok@example.com", Success: true},
		{Email: "failed@example.com", Success: false},
	}})

	if len(got) != 1 || got[0] != "ok@example.com" {
		t.Fatalf("expected only successful email, got %#v", got)
	}
}
