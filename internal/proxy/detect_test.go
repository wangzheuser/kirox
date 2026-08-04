package proxy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestDefaultDetectSelectOptionsUsesThreeShortAttempts 锁定设置页模板代理检测耗时。
func TestDefaultDetectSelectOptionsUsesThreeShortAttempts(t *testing.T) {
	opts := DefaultDetectSelectOptions()

	if opts.MaxAttempts != 3 {
		t.Fatalf("设置页检测应尝试 3 个模板代理: got %d", opts.MaxAttempts)
	}
	if opts.Timeout != 2500*time.Millisecond {
		t.Fatalf("设置页检测单次超时应为 2.5 秒: got %s", opts.Timeout)
	}
}

func TestDetectTemplatedProxySucceedsOnThirdDistinctUUIDAfter502s(t *testing.T) {
	ids := []string{
		"11111111111141118111111111111111",
		"22222222222242228222222222222222",
		"33333333333343338333333333333333",
	}
	var seen []string
	info := detectTemplatedProxyWithOptions(
		"https://user.{uuid}:pass@proxy.example.test:443",
		SelectOptions{
			MaxAttempts: 3,
			Timeout:     time.Second,
			UUIDFactory: func() string { return ids[len(seen)] },
			Check: func(_ context.Context, proxyURL, _ string, _ time.Duration) error {
				seen = append(seen, proxyURL)
				if len(seen) < 3 {
					return errors.New("Proxy responded with non 200 code: 502")
				}
				return nil
			},
		},
	)

	if !info.OK || info.Attempts != 3 || info.SuccessAttempt != 3 {
		t.Fatalf("第三次抽样应成功: %+v", info)
	}
	if len(seen) != 3 {
		t.Fatalf("抽样次数 = %d, want 3", len(seen))
	}
	for i, proxyURL := range seen {
		if !strings.Contains(proxyURL, ids[i]) {
			t.Fatalf("第 %d 次代理未使用对应 UUID: %q", i+1, proxyURL)
		}
	}
	if len(info.Errors) != 2 || !strings.Contains(info.Errors[0], "502") || !strings.Contains(info.Errors[1], "502") {
		t.Fatalf("前两次 502 应保留在详情中: %v", info.Errors)
	}
}

func TestDetectTemplatedProxyNormalizesOptionsBeforeTotalBudget(t *testing.T) {
	start := time.Now()
	var budget time.Duration
	checks := 0
	info := detectTemplatedProxyWithOptions(
		"https://user.{uuid}:pass@proxy.example.test:443",
		SelectOptions{
			Timeout: 20 * time.Millisecond,
			Check: func(ctx context.Context, _, _ string, _ time.Duration) error {
				checks++
				if checks == 1 {
					deadline, ok := ctx.Deadline()
					if !ok {
						t.Fatal("检测上下文应有总截止时间")
					}
					budget = deadline.Sub(start)
				}
				return errors.New("sample failed")
			},
		},
	)

	if info.OK || checks != defaultSelectAttempts {
		t.Fatalf("归一化后的抽样结果异常: info=%+v checks=%d", info, checks)
	}
	if budget < 80*time.Millisecond || budget > 250*time.Millisecond {
		t.Fatalf("总预算 = %s, want about %s", budget, time.Duration(defaultSelectAttempts)*20*time.Millisecond)
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
		"Proxy responded with non 200 code: 502":             "Proxy responded with non 200 code: 502",
	}

	for input, want := range cases {
		if got := simplifyProxyErr(input); got != want {
			t.Fatalf("错误文案不符合预期: input=%q got=%q want=%q", input, got, want)
		}
	}
}
