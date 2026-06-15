package data

import (
	"fmt"
	"sync"
	"testing"
)

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
	if got["priority"] != float64(9999) {
		t.Fatalf("saved account priority=%#v, want 9999", got["priority"])
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

func TestSaveKiroSuccessConcurrentUniqueEmails(t *testing.T) {
	dir := t.TempDir()
	const total = 100

	var wg sync.WaitGroup
	errCh := make(chan error, total)
	for i := 0; i < total; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- SaveKiroSuccess(map[string]interface{}{
				"status":        "success",
				"email":         fmt.Sprintf("saved-%03d@example.com", i),
				"password":      "Password123!",
				"client_id":     fmt.Sprintf("client-%03d", i),
				"client_secret": fmt.Sprintf("secret-%03d", i),
				"aws_token": map[string]interface{}{
					"accessToken":  fmt.Sprintf("access-%03d", i),
					"refreshToken": fmt.Sprintf("refresh-%03d", i),
				},
			}, dir)
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("SaveKiroSuccess returned error: %v", err)
		}
	}

	accounts, err := LoadAccounts(dir)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(accounts) != total {
		t.Fatalf("expected %d saved accounts, got %d", total, len(accounts))
	}

	seen := make(map[string]struct{}, total)
	for _, account := range accounts {
		email, _ := account["email"].(string)
		seen[email] = struct{}{}
	}
	for i := 0; i < total; i++ {
		email := fmt.Sprintf("saved-%03d@example.com", i)
		if _, ok := seen[email]; !ok {
			t.Fatalf("missing saved account %s", email)
		}
	}
}
