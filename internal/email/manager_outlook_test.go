package email

import (
	"testing"

	"reg_go/internal/storage"
)

func TestResetOutlookAccountStatusesKeepsAccountsAndClearsStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage.SetAccountsCached([]map[string]interface{}{
		{
			"email":        "used@outlook.com",
			"password":     "Password!",
			"clientId":     "client-id",
			"refreshToken": "refresh-token",
			"registered":   true,
			"success":      true,
			"registeredAt": "2026-05-20 10:11:52",
		},
		{
			"email":        "failed@outlook.com",
			"password":     "Password!",
			"clientId":     "client-id",
			"refreshToken": "refresh-token",
			"registered":   true,
			"success":      false,
			"registeredAt": "2026-05-20 10:12:52",
		},
	})

	result := ResetOutlookAccountStatuses()
	if result["reset"] != 2 {
		t.Fatalf("重置数量错误: %+v", result)
	}

	accounts := storage.GetAccountsCached()
	if len(accounts) != 2 {
		t.Fatalf("重置状态不应删除账号: got %d", len(accounts))
	}
	for _, acc := range accounts {
		if registered, _ := acc["registered"].(bool); registered {
			t.Fatalf("registered 应被重置为 false: %+v", acc)
		}
		if success, _ := acc["success"].(bool); success {
			t.Fatalf("success 应被重置为 false: %+v", acc)
		}
		if _, ok := acc["registeredAt"]; ok {
			t.Fatalf("registeredAt 应被删除: %+v", acc)
		}
	}
}
