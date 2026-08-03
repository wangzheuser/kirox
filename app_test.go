package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"reg_go/internal/data"
	"reg_go/internal/kirorsync"
)

func TestAddGoroutineLabelInsertsLabelAfterTimestamp(t *testing.T) {
	got := addGoroutineLabel("12:55:40 [Kiro] 开始\n")

	if !regexp.MustCompile(`^12:55:40 \[g\d+\] \[Kiro\] 开始\n$`).MatchString(got) {
		t.Fatalf("expected goroutine label after timestamp, got %q", got)
	}
}

func TestAddGoroutineLabelSupportsDateTimestamp(t *testing.T) {
	got := addGoroutineLabel("2026/08/03 12:55:40 [Kiro] 开始\n")

	if !regexp.MustCompile(`^2026/08/03 12:55:40 \[g\d+\] \[Kiro\] 开始\n$`).MatchString(got) {
		t.Fatalf("expected goroutine label after date timestamp, got %q", got)
	}
}

func TestRedactSensitiveLog(t *testing.T) {
	input := `邮箱 user@example.com 验证码: 123456 accessToken=access-value refreshToken:"refresh-value" password=secret-value https://proxy-user:proxy-pass@proxy.example:443`
	got := redactSensitiveLog(input)
	for _, secret := range []string{"user@example.com", "123456", "access-value", "refresh-value", "secret-value", "proxy-pass"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sensitive value %q leaked in %q", secret, got)
		}
	}
	if !strings.Contains(got, "u***@example.com") {
		t.Fatalf("masked email should retain its domain, got %q", got)
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

func TestRegistrationConcurrencyInputAllowsMaximum100(t *testing.T) {
	for _, path := range []string{"frontend/index.html", "frontend/dist/index.html"} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(content), `id="cfg-concurrency" value="1" min="1" max="100"`) {
			t.Fatalf("%s should set cfg-concurrency max to 100", path)
		}
	}
}

func TestRegistrationFormIncludesProxyExplorationPercentInput(t *testing.T) {
	content, err := os.ReadFile("frontend/index.html")
	if err != nil {
		t.Fatalf("read frontend/index.html: %v", err)
	}
	if !strings.Contains(string(content), `id="cfg-proxy-exploration-percent"`) {
		t.Fatalf("registration form should include proxy exploration percent input")
	}
}

func TestApplyKiroRSSyncResultMarksSuccessAndDeletesRejected(t *testing.T) {
	dir := t.TempDir()
	accountsJSON := `[
	  {"email":"ok@example.com","refreshToken":"ok-refresh","kiroRsSynced":false},
	  {"email":"bad@example.com","refreshToken":"bad-refresh","kiroRsSynced":false},
	  {"email":"keep@example.com","refreshToken":"keep-refresh","kiroRsSynced":false}
	]`
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(accountsJSON), 0o600); err != nil {
		t.Fatalf("write accounts.json: %v", err)
	}

	result := kirorsync.SyncResult{Details: []kirorsync.SyncDetail{
		{Email: "ok@example.com", Success: true},
		{Email: "bad@example.com", Success: false, Rejected: true, RejectReason: "凭证已被封禁或禁用"},
	}}
	updated, removed, rejectedEmails, err := applyKiroRSSyncResult(dir, result)
	if err != nil {
		t.Fatalf("applyKiroRSSyncResult: %v", err)
	}
	if updated != 1 || removed != 1 {
		t.Fatalf("updated=%d removed=%d, want 1/1", updated, removed)
	}
	if len(rejectedEmails) != 1 || rejectedEmails[0] != "bad@example.com" {
		t.Fatalf("unexpected rejected emails: %#v", rejectedEmails)
	}

	accounts, err := data.LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected rejected account removed, got %#v", accounts)
	}
	var okSynced bool
	for _, acc := range accounts {
		email, _ := acc["email"].(string)
		if email == "bad@example.com" {
			t.Fatalf("rejected account should be deleted: %#v", accounts)
		}
		if email == "ok@example.com" {
			okSynced, _ = acc["kiroRsSynced"].(bool)
		}
	}
	if !okSynced {
		t.Fatalf("successful account should be marked kiroRsSynced: %#v", accounts)
	}
}
