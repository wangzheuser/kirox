package data

import "testing"

func TestSaveKiroSuccessPersistsPasswordAccessTokenAndPriority(t *testing.T) {
	dir := t.TempDir()

	err := SaveKiroSuccess(map[string]interface{}{
		"status":        "success",
		"email":         "saved@example.com",
		"password":      "Password123!",
		"client_id":     "client-id",
		"client_secret": "client-secret",
		"aws_token": map[string]interface{}{
			"accessToken":  "access-token",
			"refreshToken": "refresh-token",
		},
	}, dir)
	if err != nil {
		t.Fatalf("SaveKiroSuccess returned error: %v", err)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected one saved account, got %d", len(accounts))
	}
	got := accounts[0]
	if got["password"] != "Password123!" ||
		got["accessToken"] != "access-token" ||
		got["refreshToken"] != "refresh-token" ||
		got["clientId"] != "client-id" ||
		got["clientSecret"] != "client-secret" {
		t.Fatalf("saved account missing reference export fields: %#v", got)
	}
	if got["priority"] == nil {
		t.Fatalf("saved account should include calculated priority: %#v", got)
	}
	if synced, ok := got["kiroRsSynced"].(bool); !ok || synced {
		t.Fatalf("newly saved account should default kiroRsSynced=false: %#v", got)
	}
}

func TestSaveKiroSuccessResetsKiroRSSyncStatusWhenRefreshTokenChanges(t *testing.T) {
	dir := t.TempDir()

	first := map[string]interface{}{
		"status":        "success",
		"email":         "saved@example.com",
		"password":      "Password123!",
		"client_id":     "client-id",
		"client_secret": "client-secret",
		"aws_token": map[string]interface{}{
			"accessToken":  "access-token",
			"refreshToken": "old-refresh-token",
		},
	}
	if err := SaveKiroSuccess(first, dir); err != nil {
		t.Fatalf("initial SaveKiroSuccess: %v", err)
	}
	if updated, err := MarkKiroRSSynced(dir, []string{"saved@example.com"}); err != nil || updated != 1 {
		t.Fatalf("MarkKiroRSSynced updated=%d err=%v", updated, err)
	}

	second := map[string]interface{}{
		"status":        "success",
		"email":         "saved@example.com",
		"password":      "Password123!",
		"client_id":     "client-id",
		"client_secret": "client-secret",
		"aws_token": map[string]interface{}{
			"accessToken":  "new-access-token",
			"refreshToken": "new-refresh-token",
		},
	}
	if err := SaveKiroSuccess(second, dir); err != nil {
		t.Fatalf("second SaveKiroSuccess: %v", err)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	got := accountByEmail(t, accounts, "saved@example.com")
	if got["refreshToken"] != "new-refresh-token" {
		t.Fatalf("refresh token was not replaced: %#v", got)
	}
	if synced, ok := got["kiroRsSynced"].(bool); !ok || synced {
		t.Fatalf("changed refresh token should reset kiroRsSynced=false: %#v", got)
	}
}

func TestSaveKiroSuccessPreservesKiroRSSyncedWhenRefreshTokenUnchanged(t *testing.T) {
	dir := t.TempDir()
	result := map[string]interface{}{
		"status":        "success",
		"email":         "saved@example.com",
		"password":      "Password123!",
		"client_id":     "client-id",
		"client_secret": "client-secret",
		"aws_token": map[string]interface{}{
			"accessToken":  "access-token",
			"refreshToken": "same-refresh-token",
		},
	}
	if err := SaveKiroSuccess(result, dir); err != nil {
		t.Fatalf("initial SaveKiroSuccess: %v", err)
	}
	if updated, err := MarkKiroRSSynced(dir, []string{"saved@example.com"}); err != nil || updated != 1 {
		t.Fatalf("MarkKiroRSSynced updated=%d err=%v", updated, err)
	}
	if err := SaveKiroSuccess(result, dir); err != nil {
		t.Fatalf("second SaveKiroSuccess: %v", err)
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	got := accountByEmail(t, accounts, "saved@example.com")
	if synced, ok := got["kiroRsSynced"].(bool); !ok || !synced {
		t.Fatalf("unchanged refresh token should preserve kiroRsSynced=true: %#v", got)
	}
}
