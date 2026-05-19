package proxy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSelectRuntimeProxyTemplateSucceedsOnThirdCandidate(t *testing.T) {
	uuids := []string{"uuid-1", "uuid-2", "uuid-3"}
	calls := 0

	selection, err := SelectRuntimeProxy(context.Background(),
		"https://node.{uuid}:admin2012@proxy.example.com:443",
		SelectOptions{
			MaxAttempts: 3,
			Timeout:     time.Second,
			TargetURL:   "https://example.com/probe",
			UUIDFactory: func() string {
				id := uuids[calls]
				calls++
				return id
			},
			Check: func(_ context.Context, proxyURL, _ string, _ time.Duration) error {
				if strings.Contains(proxyURL, "uuid-3") {
					return nil
				}
				return errors.New("dial timeout")
			},
		})

	if err != nil {
		t.Fatalf("期望第三个候选成功，实际失败: %v", err)
	}
	if selection.ProxyURL != "https://node.uuid-3:admin2012@proxy.example.com:443" {
		t.Fatalf("运行时代理不符合预期: %q", selection.ProxyURL)
	}
	if selection.SuccessAttempt != 3 || selection.Attempts != 3 {
		t.Fatalf("候选次数不符合预期: success=%d attempts=%d", selection.SuccessAttempt, selection.Attempts)
	}
	if calls != 3 {
		t.Fatalf("UUID 生成次数不符合预期: %d", calls)
	}
}

func TestSelectRuntimeProxyFailureDoesNotLeakPassword(t *testing.T) {
	selection, err := SelectRuntimeProxy(context.Background(),
		"https://node.{uuid}:admin2012@proxy.example.com:443",
		SelectOptions{
			MaxAttempts: 2,
			Timeout:     time.Second,
			UUIDFactory: func() string { return "11111111-2222-4333-8444-555555555555" },
			Check: func(_ context.Context, proxyURL, _ string, _ time.Duration) error {
				return errors.New("failed via " + proxyURL)
			},
		})

	if err == nil {
		t.Fatal("期望所有候选失败")
	}
	combined := err.Error() + strings.Join(selection.Errors, " ")
	if strings.Contains(combined, "admin2012") || strings.Contains(combined, "11111111-2222-4333-8444-555555555555") {
		t.Fatalf("错误信息泄漏了代理凭据或 UUID: %s", combined)
	}
	if !strings.Contains(combined, "node.%3Cuuid%3E") && !strings.Contains(combined, "node.<uuid>") {
		t.Fatalf("错误信息应包含脱敏用户名: %s", combined)
	}
}

func TestSelectRuntimeProxyPlainProxyDoesNotGenerateUUID(t *testing.T) {
	calls := 0

	selection, err := SelectRuntimeProxy(context.Background(),
		"https://user:pass@proxy.example.com:443",
		SelectOptions{
			MaxAttempts: 5,
			UUIDFactory: func() string {
				calls++
				return "unused"
			},
			Check: func(_ context.Context, _ string, _ string, _ time.Duration) error {
				return nil
			},
		})

	if err != nil {
		t.Fatalf("普通代理不应失败: %v", err)
	}
	if calls != 0 {
		t.Fatalf("普通代理不应生成 UUID: %d", calls)
	}
	if selection.Attempts != 1 || selection.Templated {
		t.Fatalf("普通代理应只检测一次且非模板: %+v", selection)
	}
}

func TestMaskURLDoesNotLeakCredentials(t *testing.T) {
	got := MaskURL("https://node.11111111-2222-4333-8444-555555555555:admin2012@proxy.example.com:443")

	if strings.Contains(got, "admin2012") || strings.Contains(got, "11111111-2222-4333-8444-555555555555") {
		t.Fatalf("脱敏代理泄漏敏感信息: %s", got)
	}
	if !strings.Contains(got, "proxy.example.com:443") {
		t.Fatalf("脱敏代理应保留主机端口: %s", got)
	}
}
