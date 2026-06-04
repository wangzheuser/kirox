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
