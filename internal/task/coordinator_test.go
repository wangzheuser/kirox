package task

import (
	"testing"
	"time"

	"reg_go/internal/kirorsync"
)

func TestConcurrentStartStaggerDisabledForSerial(t *testing.T) {
	if got := concurrentStartStagger(0, 1); got != 0 {
		t.Fatalf("serial stagger should be 0, got %s", got)
	}
}

func TestConcurrentStartStaggerSpreadsInitialConcurrencyWindow(t *testing.T) {
	cases := []struct {
		idx      int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{idx: 0, minDelay: 0, maxDelay: concurrentStartStaggerJitterMax},
		{idx: 1, minDelay: 100 * time.Millisecond, maxDelay: 100*time.Millisecond + concurrentStartStaggerJitterMax},
		{idx: 9, minDelay: 900 * time.Millisecond, maxDelay: 900*time.Millisecond + concurrentStartStaggerJitterMax},
		{idx: 10, minDelay: 0, maxDelay: concurrentStartStaggerJitterMax},
	}

	for _, tc := range cases {
		got := concurrentStartStagger(tc.idx, 10)
		if got < tc.minDelay || got > tc.maxDelay {
			t.Fatalf("idx %d stagger = %s, want within [%s, %s]", tc.idx, got, tc.minDelay, tc.maxDelay)
		}
	}
}

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

func TestSelectAccountsByEmailReportsMissingBatchEmails(t *testing.T) {
	accounts := []map[string]interface{}{
		{"email": "saved@example.com"},
	}

	selected, missing := selectAccountsByEmail(accounts, []string{"saved@example.com", "missing@example.com"})

	if len(selected) != 1 || selected[0]["email"] != "saved@example.com" {
		t.Fatalf("expected saved account selected, got %#v", selected)
	}
	if len(missing) != 1 || missing[0] != "missing@example.com" {
		t.Fatalf("expected missing email reported, got %#v", missing)
	}
}
