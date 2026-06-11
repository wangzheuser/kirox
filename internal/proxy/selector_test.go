package proxy

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSelectRuntimeProxyTemplateSucceedsOnThirdCandidate(t *testing.T) {
	uuids := []string{
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		"33333333-3333-4333-8333-333333333333",
	}
	wantID := "33333333333343338333333333333333"
	calls := 0

	selection, err := SelectRuntimeProxy(context.Background(),
		"http://node.{uuid}:template-pass@proxy.example.test:443",
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
				if strings.Contains(proxyURL, wantID) {
					return nil
				}
				return errors.New("dial timeout")
			},
		})

	if err != nil {
		t.Fatalf("期望第三个候选成功，实际失败: %v", err)
	}
	if selection.ProxyURL != "http://node."+wantID+":template-pass@proxy.example.test:443" {
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
	fixedUUID := "11111111-2222-4333-8444-555555555555"
	normalizedUUID := "11111111222243338444555555555555"
	selection, err := SelectRuntimeProxy(context.Background(),
		"http://node.{uuid}:template-pass@proxy.example.test:443",
		SelectOptions{
			MaxAttempts: 2,
			Timeout:     time.Second,
			UUIDFactory: func() string { return fixedUUID },
			Check: func(_ context.Context, proxyURL, _ string, _ time.Duration) error {
				return errors.New("failed via " + proxyURL)
			},
		})

	if err == nil {
		t.Fatal("期望所有候选失败")
	}
	combined := err.Error() + strings.Join(selection.Errors, " ")
	if strings.Contains(combined, "template-pass") || strings.Contains(combined, fixedUUID) || strings.Contains(combined, normalizedUUID) {
		t.Fatalf("错误信息泄漏了代理凭据或 UUID: %s", combined)
	}
	if !strings.Contains(combined, "node.%3Cuuid%3E") && !strings.Contains(combined, "node.<uuid>") {
		t.Fatalf("错误信息应包含脱敏用户名: %s", combined)
	}
}

func TestSelectRuntimeProxyPlainProxyDoesNotGenerateUUID(t *testing.T) {
	calls := 0

	selection, err := SelectRuntimeProxy(context.Background(),
		"http://127.0.0.1:7890",
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
	got := MaskURL("http://proxy-user:template-pass@proxy.example.test:443")

	if strings.Contains(got, "template-pass") || strings.Contains(got, "11111111-2222-4333-8444-555555555555") {
		t.Fatalf("脱敏代理泄漏敏感信息: %s", got)
	}
	if !strings.Contains(got, "proxy.example.test:443") {
		t.Fatalf("脱敏代理应保留主机端口: %s", got)
	}
}

func TestMaskURLSupportsTemplateUserInfo(t *testing.T) {
	got := MaskURL("https://node.{uuid}:template-pass@proxy.example.test:443")

	if strings.Contains(got, "template-pass") || strings.Contains(got, "{uuid}") || strings.Contains(got, "00000000000000000000000000000000") {
		t.Fatalf("模板代理脱敏结果泄漏敏感信息: %s", got)
	}
	if !strings.Contains(got, "https://node.<uuid>:***@proxy.example.test:443") {
		t.Fatalf("模板代理脱敏格式异常: %s", got)
	}
}

func TestSelectRuntimeProxyDefaultUUIDHasNoHyphen(t *testing.T) {
	_, err := SelectRuntimeProxy(context.Background(),
		"https://node.{uuid}:template-pass@proxy.example.test:443",
		SelectOptions{
			Check: func(_ context.Context, proxyURL, _ string, _ time.Duration) error {
				parsed, err := url.Parse(proxyURL)
				if err != nil {
					t.Fatalf("渲染后的代理 URL 应可解析: %v", err)
				}
				username := parsed.User.Username()
				if strings.Contains(username, "{uuid}") || strings.Contains(username, "-") {
					t.Fatalf("默认 UUID 应完成渲染且不包含短横线: %q", username)
				}
				return nil
			},
		})

	if err != nil {
		t.Fatalf("默认 UUID 模板代理不应失败: %v", err)
	}
}
