package task

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"reg_go/internal/storage"
)

func TestEmailProviderSelectorRoundRobinsSerially(t *testing.T) {
	selector := newEmailProviderSelector([]string{"emailnator", "mailgw", "dropmail"})

	got := make([]string, 0, 7)
	for i := 0; i < 7; i++ {
		got = append(got, selector.Next())
	}

	want := []string{"emailnator", "mailgw", "dropmail", "emailnator", "mailgw", "dropmail", "emailnator"}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("round robin mismatch at %d: got %#v, want %#v", i, got, want)
		}
	}
}

func TestEmailProviderSelectorRoundRobinsConcurrently(t *testing.T) {
	selector := newEmailProviderSelector([]string{"emailnator", "mailgw", "dropmail"})

	const total = 300
	results := make([]string, total)
	var wg sync.WaitGroup
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = selector.Next()
		}(i)
	}
	wg.Wait()

	counts := map[string]int{}
	for _, provider := range results {
		counts[provider]++
	}
	if counts["emailnator"] != 100 || counts["mailgw"] != 100 || counts["dropmail"] != 100 {
		t.Fatalf("concurrent round robin counts mismatch: %#v", counts)
	}
}

func TestNormalizeStartEmailProvidersDeduplicatesAndDefaults(t *testing.T) {
	got, err := normalizeStartEmailProviders([]string{" emailnator ", "mailgw", "emailnator", ""})
	if err != nil {
		t.Fatalf("normalizeStartEmailProviders returned error: %v", err)
	}
	want := []string{"emailnator", "mailgw"}
	if len(got) != len(want) {
		t.Fatalf("len=%d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("providers mismatch: got %#v, want %#v", got, want)
		}
	}

	defaulted, err := normalizeStartEmailProviders(nil)
	if err != nil {
		t.Fatalf("default normalize returned error: %v", err)
	}
	if len(defaulted) != 1 || defaulted[0] != "outlook" {
		t.Fatalf("empty provider list should default to outlook, got %#v", defaulted)
	}
}

func TestNormalizeStartEmailProvidersRejectsInvalidProvider(t *testing.T) {
	if _, err := normalizeStartEmailProviders([]string{"emailnator", "invalid"}); err == nil {
		t.Fatalf("invalid provider should be rejected")
	}
}

func TestEmailProviderSelectorSetIsImmutable(t *testing.T) {
	providers := []string{"emailnator", "mailgw"}
	selector := newEmailProviderSelector(providers)
	providers[0] = "dropmail"

	got := []string{selector.Next(), selector.Next(), selector.Next()}
	sort.Strings(got[:2])
	if got[0] != "emailnator" || got[1] != "mailgw" || got[2] != "emailnator" {
		t.Fatalf("selector should keep an internal copy, got %#v", got)
	}
}

func TestEmailProviderSelectorCreateFailureCooldown(t *testing.T) {
	selector := newEmailProviderSelector([]string{"mailgw"})
	for i := 1; i <= emailProviderCreateFailureCooldownThreshold; i++ {
		cooled := selector.ReportCreateFailure("mailgw")
		if cooled != (i == emailProviderCreateFailureCooldownThreshold) {
			t.Fatalf("failure %d cooled=%v", i, cooled)
		}
	}

	if provider, wait, ok := selector.nextAvailable(time.Now()); ok || provider != "" || wait <= 0 {
		t.Fatalf("create-failed provider should cool: provider=%q wait=%s ok=%v", provider, wait, ok)
	}
	selector.ReportCreateSuccess("mailgw")
	if provider, _, ok := selector.nextAvailable(time.Now()); !ok || provider != "mailgw" {
		t.Fatalf("create success should clear create cooldown: provider=%q ok=%v", provider, ok)
	}
}

func TestEmailProviderSelectorMailboxCooldownSurvivesCreateSuccess(t *testing.T) {
	selector := newEmailProviderSelector([]string{"mailgw"})
	result := map[string]interface{}{
		"status":        "failed",
		"formSubmitted": true,
		"otpSent":       true,
		"otpReceived":   false,
		"error":         "验证码接收超时，请检查邮箱服务或稍后重试",
	}
	for i := 1; i <= emailProviderMailboxFailureCooldownThreshold; i++ {
		cooled := selector.ReportMailboxResult("mailgw", result)
		if cooled != (i == emailProviderMailboxFailureCooldownThreshold) {
			t.Fatalf("mailbox failure %d cooled=%v", i, cooled)
		}
	}

	selector.ReportCreateSuccess("mailgw")
	if provider, wait, ok := selector.nextAvailable(time.Now()); ok || provider != "" || wait <= 0 {
		t.Fatalf("create success must not clear mailbox cooldown: provider=%q wait=%s ok=%v", provider, wait, ok)
	}

	selector.ReportMailboxResult("mailgw", map[string]interface{}{
		"status":        "failed",
		"formSubmitted": true,
		"otpReceived":   true,
	})
	if provider, _, ok := selector.nextAvailable(time.Now()); !ok || provider != "mailgw" {
		t.Fatalf("OTP received should clear mailbox cooldown: provider=%q ok=%v", provider, ok)
	}
}

func TestEmailProviderSelectorDoesNotPenalizeProxyOrPreFormFailure(t *testing.T) {
	selector := newEmailProviderSelector([]string{"mailgw"})
	results := []map[string]interface{}{
		{
			"status":        "failed",
			"formSubmitted": true,
			"otpReceived":   false,
			"error":         "proxy connect timeout while waiting for mailbox",
		},
		{
			"status":        "failed",
			"formSubmitted": false,
			"otpReceived":   false,
			"error":         "验证码接收超时，请检查邮箱服务或稍后重试",
		},
	}
	for _, result := range results {
		for i := 0; i < emailProviderMailboxFailureCooldownThreshold+1; i++ {
			if selector.ReportMailboxResult("mailgw", result) {
				t.Fatalf("non-mailbox result should not trigger cooldown: %#v", result)
			}
		}
	}
	if provider, _, ok := selector.nextAvailable(time.Now()); !ok || provider != "mailgw" {
		t.Fatalf("provider should remain available: provider=%q ok=%v", provider, ok)
	}
}

func TestEmailProviderSelectorCooldownWaitIsCancellable(t *testing.T) {
	selector := newEmailProviderSelector([]string{"mailgw"})
	selector.mailboxCooldownUntil["mailgw"] = time.Now().Add(emailProviderMailboxFailureCooldown)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := selector.NextWithContext(ctx)
		done <- err
	}()
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("NextWithContext error=%v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("NextWithContext did not stop after context cancellation")
	}
}

func TestIsEmailProviderMailboxFailure(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want bool
	}{
		{name: "send otp rejected", err: `发送验证码失败: send-otp 失败 (400): rejected`, want: true},
		{name: "send otp forbidden", err: `发送验证码失败: send-otp 失败 (403): rejected`, want: true},
		{name: "TES blocked", err: `注册被拦截: 请更换IP或稍后重试 [provider=mailgw, domain=example.test]`, want: true},
		{name: "OTP timeout", err: "验证码接收超时，请检查邮箱服务或稍后重试", want: true},
		{name: "mailbox read", err: "邮箱服务异常，无法接收验证码", want: true},
		{name: "send otp rate limit", err: `发送验证码失败: send-otp 失败 (429): rate limited`, want: false},
		{name: "send otp upstream", err: `发送验证码失败: send-otp 失败 (503): unavailable`, want: false},
		{name: "proxy timeout", err: "proxy connect timeout", want: false},
		{name: "cancelled", err: "任务已取消", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEmailProviderMailboxFailure(tt.err); got != tt.want {
				t.Fatalf("isEmailProviderMailboxFailure(%q)=%v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestEffectiveEmailProvidersForBatchOrdersHistoricalWinnerFirstWithoutDroppingSelected(t *testing.T) {
	got := effectiveEmailProvidersForBatchByStats(
		[]string{"pickmail", "blinkbox"},
		[]storage.EmailProviderStat{
			{Provider: "pickmail", OTPReceivedCount: 8, RegistrationSuccessCount: 1},
			{Provider: "blinkbox", OTPReceivedCount: 16, RegistrationSuccessCount: 11},
		},
	)

	if len(got) != 2 || got[0] != "blinkbox" || got[1] != "pickmail" {
		t.Fatalf("historical winner should be ordered first without dropping selected providers, got %#v", got)
	}
}

func TestEffectiveEmailProvidersForBatchKeepsOriginalWithoutStrongWinner(t *testing.T) {
	providers := []string{"emailnator", "mailgw"}
	got := effectiveEmailProvidersForBatchByStats(
		providers,
		[]storage.EmailProviderStat{
			{Provider: "emailnator", OTPReceivedCount: 2, RegistrationSuccessCount: 2},
			{Provider: "mailgw", OTPReceivedCount: 6, RegistrationSuccessCount: 2},
		},
	)

	if len(got) != len(providers) || got[0] != providers[0] || got[1] != providers[1] {
		t.Fatalf("providers without a strong historical winner should keep original order, got %#v", got)
	}
	providers[0] = "changed"
	if got[0] != "emailnator" {
		t.Fatalf("returned provider list should not alias input, got %#v", got)
	}
}
