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

func TestMarkAccountFailReasonKeepsFirstOTPTimeoutRetryable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage.SetAccountsCached([]map[string]interface{}{
		{
			"email":        "timeout-once@outlook.com",
			"password":     "Password!",
			"clientId":     "client-id",
			"refreshToken": "refresh-token",
			"registered":   false,
			"success":      false,
		},
	})

	result := MarkAccountFailReason("timeout-once@outlook.com", "验证码超时")
	if result["status"] != "updated" {
		t.Fatalf("记录失败原因失败: %+v", result)
	}

	acc := storage.GetAccountsCached()[0]
	if registered, _ := acc["registered"].(bool); registered {
		t.Fatalf("首次验证码超时不应标记为已占用: %+v", acc)
	}
	if got, _ := acc["failReason"].(string); got != "验证码超时" {
		t.Fatalf("首次验证码超时应记录失败原因，got %q: %+v", got, acc)
	}
}

func TestMarkAccountFailReasonMarksAbnormalAfterConsecutiveOTPTimeouts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage.SetAccountsCached([]map[string]interface{}{
		{
			"email":        "timeout-twice@outlook.com",
			"password":     "Password!",
			"clientId":     "client-id",
			"refreshToken": "refresh-token",
			"registered":   false,
			"success":      false,
			"failReason":   "验证码超时",
		},
	})

	result := MarkAccountFailReason("timeout-twice@outlook.com", "验证码超时")
	if result["status"] != "updated" {
		t.Fatalf("记录失败原因失败: %+v", result)
	}

	acc := storage.GetAccountsCached()[0]
	if registered, _ := acc["registered"].(bool); !registered {
		t.Fatalf("连续两次验证码超时应标记为已占用: %+v", acc)
	}
	if success, _ := acc["success"].(bool); success {
		t.Fatalf("异常邮箱不应标记为成功: %+v", acc)
	}
	if got, _ := acc["failReason"].(string); got != "异常邮箱" {
		t.Fatalf("连续两次验证码超时应标记为异常邮箱，got %q: %+v", got, acc)
	}
	if _, ok := acc["registeredAt"]; !ok {
		t.Fatalf("异常邮箱应记录 registeredAt: %+v", acc)
	}
}

func TestMarkAccountFailReasonOtherReasonBreaksConsecutiveOTPTimeout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage.SetAccountsCached([]map[string]interface{}{
		{
			"email":        "timeout-after-network@outlook.com",
			"password":     "Password!",
			"clientId":     "client-id",
			"refreshToken": "refresh-token",
			"registered":   false,
			"success":      false,
			"failReason":   "网络/代理问题",
		},
	})

	result := MarkAccountFailReason("timeout-after-network@outlook.com", "验证码超时")
	if result["status"] != "updated" {
		t.Fatalf("记录失败原因失败: %+v", result)
	}

	acc := storage.GetAccountsCached()[0]
	if registered, _ := acc["registered"].(bool); registered {
		t.Fatalf("非连续验证码超时不应标记为已占用: %+v", acc)
	}
	if got, _ := acc["failReason"].(string); got != "验证码超时" {
		t.Fatalf("应将当前失败原因更新为验证码超时，got %q: %+v", got, acc)
	}
}

func TestAbnormalOutlookAccountMatchesRegisteredExclusionState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage.SetAccountsCached([]map[string]interface{}{
		{
			"email":        "abnormal@outlook.com",
			"password":     "Password!",
			"clientId":     "client-id",
			"refreshToken": "refresh-token",
			"registered":   false,
			"success":      false,
			"failReason":   "验证码超时",
		},
	})

	MarkAccountFailReason("abnormal@outlook.com", "验证码超时")

	acc := storage.GetAccountsCached()[0]
	if registered, _ := acc["registered"].(bool); !registered {
		t.Fatalf("异常邮箱必须使用 registered=true 才能被注册筛选排除: %+v", acc)
	}
}

func TestResetOutlookAccountStatusesByEmailsOnlyResetsTargetsAndKeepsCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	storage.SetAccountsCached([]map[string]interface{}{
		{
			"email":        "target@outlook.jp",
			"password":     "Password!",
			"clientId":     "client-id",
			"refreshToken": "refresh-token",
			"registered":   true,
			"success":      true,
			"registeredAt": "2026-05-20 10:11:52",
			"failReason":   "邮箱已注册",
		},
		{
			"email":        "keep@outlook.com",
			"password":     "KeepPassword!",
			"clientId":     "keep-client",
			"refreshToken": "keep-refresh",
			"registered":   true,
			"success":      true,
			"registeredAt": "2026-05-20 10:12:52",
			"failReason":   "邮箱已注册",
		},
	})

	result := ResetOutlookAccountStatusesByEmails([]string{"TARGET@outlook.jp", "missing@outlook.jp"})
	if result["reset"] != 1 {
		t.Fatalf("重置数量错误: %+v", result)
	}

	accounts := storage.GetAccountsCached()
	target := accounts[0]
	if registered, _ := target["registered"].(bool); registered {
		t.Fatalf("目标账号 registered 应被重置: %+v", target)
	}
	if success, _ := target["success"].(bool); success {
		t.Fatalf("目标账号 success 应被重置: %+v", target)
	}
	if _, ok := target["registeredAt"]; ok {
		t.Fatalf("目标账号 registeredAt 应被删除: %+v", target)
	}
	if _, ok := target["failReason"]; ok {
		t.Fatalf("目标账号 failReason 应被删除: %+v", target)
	}
	if target["password"] != "Password!" || target["clientId"] != "client-id" || target["refreshToken"] != "refresh-token" {
		t.Fatalf("目标账号凭据字段不应被清空: %+v", target)
	}

	keep := accounts[1]
	if registered, _ := keep["registered"].(bool); !registered {
		t.Fatalf("非目标账号不应被重置: %+v", keep)
	}
	if keep["password"] != "KeepPassword!" || keep["clientId"] != "keep-client" || keep["refreshToken"] != "keep-refresh" {
		t.Fatalf("非目标账号凭据字段不应被改变: %+v", keep)
	}
}

func TestSaveOutlookGraphResolutionPersistsByEmailCaseInsensitive(t *testing.T) {
	storage.SetAccountsCached([]map[string]interface{}{
		{"email": "Alias@outlook.jp", "password": "p", "clientId": "c", "refreshToken": "r", "registered": false},
	})

	result := SaveOutlookGraphResolution("alias@OUTLOOK.jp", OutlookAccount{
		Email:              "Alias@outlook.jp",
		RegistrationEmail:  "Alias@outlook.jp",
		GraphPrimaryEmail:  "primary@hotmail.com",
		GraphAliasVerified: true,
	})
	if result["status"] != "updated" {
		t.Fatalf("SaveOutlookGraphResolution result=%v", result)
	}
	accounts := GetOutlookAccounts()
	acc := accounts[0]
	if acc["registrationEmail"] != "Alias@outlook.jp" || acc["graphPrimaryEmail"] != "primary@hotmail.com" || acc["graphAliasVerified"] != true {
		t.Fatalf("graph fields not persisted: %+v", acc)
	}
	if _, ok := acc["graphResolvedAt"].(string); !ok {
		t.Fatalf("graphResolvedAt should be persisted: %+v", acc)
	}
}

func TestDeleteOutlookAccountsByFailReasonDeletesCaseInsensitiveReason(t *testing.T) {
	storage.SetAccountsCached([]map[string]interface{}{
		{"email": "registered@outlook.jp", "failReason": "邮箱已注册", "registered": true, "success": false},
		{"email": "keep@outlook.jp", "failReason": "验证码超时", "registered": false, "success": false},
	})

	result := DeleteOutlookAccountsByFailReason("邮箱已注册")
	if result["removed"] != 1 {
		t.Fatalf("expected one removed account, got %+v", result)
	}
	accounts := storage.GetAccountsCached()
	if len(accounts) != 1 || accounts[0]["email"] != "keep@outlook.jp" {
		t.Fatalf("unexpected remaining accounts: %#v", accounts)
	}
}
