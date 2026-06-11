package proxy

import (
	"context"
	"testing"
	"time"
)

// TestDefaultDetectSelectOptionsUsesSingleShortAttempt 锁定设置页模板代理检测耗时。
func TestDefaultDetectSelectOptionsUsesSingleShortAttempt(t *testing.T) {
	opts := DefaultDetectSelectOptions()

	if opts.MaxAttempts != 1 {
		t.Fatalf("设置页检测应只尝试 1 个模板代理: got %d", opts.MaxAttempts)
	}
	if opts.Timeout != 8*time.Second {
		t.Fatalf("设置页检测单次超时应为 8 秒: got %s", opts.Timeout)
	}
}

// TestDetectTemplatedProxyTimeoutReturns 验证底层检测阻塞时设置页能按时返回。
func TestDetectTemplatedProxyTimeoutReturns(t *testing.T) {
	start := time.Now()
	info := detectTemplatedProxyWithOptions(
		"https://user.{uuid}:pass@proxy.example.test:443",
		SelectOptions{
			MaxAttempts: 1,
			Timeout:     20 * time.Millisecond,
			TargetURL:   "https://example.com/ping",
			Check: func(ctx context.Context, _, _ string, _ time.Duration) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
	)

	if info.OK {
		t.Fatal("阻塞检测不应成功")
	}
	if info.Error != "检测超时" {
		t.Fatalf("检测超时错误不符合预期: %q", info.Error)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("检测返回耗时过长: %s", elapsed)
	}
}

// TestSimplifyProxyErrReadableMessages 验证代理错误文案对用户可读。
func TestSimplifyProxyErrReadableMessages(t *testing.T) {
	cases := map[string]string{
		"context deadline exceeded":                          "检测超时",
		"Client.Timeout exceeded while awaiting headers":     "检测超时",
		"HTTPS 代理握手失败: remote error: tls: handshake failure": "HTTPS 代理握手失败，请确认代理服务是否支持 HTTPS 代理协议",
		"代理 CONNECT 失败: 407 Proxy Authentication Required":   "代理 CONNECT 失败: 407 Proxy Authentication Required",
	}

	for input, want := range cases {
		if got := simplifyProxyErr(input); got != want {
			t.Fatalf("错误文案不符合预期: input=%q got=%q want=%q", input, got, want)
		}
	}
}
