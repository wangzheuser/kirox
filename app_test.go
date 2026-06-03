package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestAddGoroutineLabelInsertsLabelAfterTimestamp(t *testing.T) {
	got := addGoroutineLabel("12:55:40 [Kiro] 开始\n")

	if !regexp.MustCompile(`^12:55:40 \[g\d+\] \[Kiro\] 开始\n$`).MatchString(got) {
		t.Fatalf("expected goroutine label after timestamp, got %q", got)
	}
}

func TestAddGoroutineLabelPrefixesLinesWithoutTimestamp(t *testing.T) {
	got := addGoroutineLabel("plain message\n")

	if !regexp.MustCompile(`^\[g\d+\] plain message\n$`).MatchString(got) {
		t.Fatalf("expected goroutine label prefix for plain line, got %q", got)
	}
}

func TestAddGoroutineLabelUsesSameLabelForMultilineLogs(t *testing.T) {
	got := addGoroutineLabel("12:55:40 first\nsecond\n")
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two lines, got %d: %q", len(lines), got)
	}

	m := regexp.MustCompile(`^12:55:40 (\[g\d+\]) first$`).FindStringSubmatch(lines[0])
	if len(m) != 2 {
		t.Fatalf("expected first line to contain timestamp goroutine label, got %q", lines[0])
	}
	if lines[1] != m[1]+" second" {
		t.Fatalf("expected second line to reuse %s, got %q", m[1], lines[1])
	}
}

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
