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
}
