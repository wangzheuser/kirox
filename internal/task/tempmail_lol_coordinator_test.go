package task

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"reg_go/internal/core"
)

func TestTempMailLOLDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProviders: []string{"tempmail_lol"}})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("tempmail_lol should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestReusableEmailSupportsTempMailLOLProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@random.example"}
	pool.put(reusableEmailCandidate{provider: "tempmail_lol", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("tempmail_lol")
	if !ok {
		t.Fatalf("expected tempmail_lol reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("tempmail_lol", &cfg, candidate)
	if !applied || address != service.address || cfg.TempEmailService != service {
		t.Fatalf("tempmail_lol reusable candidate not applied, address=%q applied=%v", address, applied)
	}
}

func TestTempMailLOLSendOTP400DoesNotTriggerKillSwitch(t *testing.T) {
	if isKillSwitchError("send-otp 失败 (400): domain rejected", "tempmail_lol") {
		t.Fatalf("tempmail_lol send-otp 400 should be treated as single mailbox failure")
	}
	if !isKillSwitchError("注册请求被拦截 BLOCKED", "tempmail_lol") {
		t.Fatalf("tempmail_lol BLOCKED/IP risk should still trigger kill switch")
	}
}

func TestTempMailLOLCreate429IsProviderRateLimit(t *testing.T) {
	errText := `创建 TempMail.lol 邮箱 HTTP 429: {"error":"Rate limited (free)"}`
	if !isEmailProviderRateLimitError(errText) {
		t.Fatalf("TempMail.lol HTTP 429 should be treated as provider rate limit")
	}
}

func TestOrdinaryTempMailLOLCreateFailureIsNotProviderRateLimit(t *testing.T) {
	errText := "创建 TempMail.lol 邮箱 HTTP 500: temporary upstream error"
	if isEmailProviderRateLimitError(errText) {
		t.Fatalf("ordinary TempMail.lol create failure should not be treated as provider rate limit")
	}
}

func TestEmailProviderRateLimitBackoffSequence(t *testing.T) {
	want := []time.Duration{60 * time.Second, 120 * time.Second, 300 * time.Second}
	for attempt, expected := range want {
		if got := emailProviderRateLimitBackoff(attempt); got != expected {
			t.Fatalf("attempt %d backoff=%s, want %s", attempt, got, expected)
		}
	}
	if got := emailProviderRateLimitBackoff(len(want)); got != 0 {
		t.Fatalf("attempt after max retries should stop, got %s", got)
	}
}

func TestEmailProviderCreateRetryDecision(t *testing.T) {
	retry, wait := shouldRetryEmailProviderCreateError(`创建 TempMail.lol 邮箱 HTTP 429: {"error":"Rate limited (free)"}`, 0)
	if !retry || wait != 60*time.Second {
		t.Fatalf("first rate limit should retry after 60s, retry=%v wait=%s", retry, wait)
	}

	retry, wait = shouldRetryEmailProviderCreateError("创建 TempMail.lol 邮箱 HTTP 500", 0)
	if retry || wait != 0 {
		t.Fatalf("ordinary create failure should not use rate-limit retry, retry=%v wait=%s", retry, wait)
	}

	retry, wait = shouldRetryEmailProviderCreateError(`创建 TempMail.lol 邮箱 HTTP 429: {"error":"Rate limited (free)"}`, 3)
	if retry || wait != 0 {
		t.Fatalf("rate limit after max retries should stop, retry=%v wait=%s", retry, wait)
	}
}

func TestCreateTempEmailWithRateLimitRetryRetriesThenSucceeds(t *testing.T) {
	orig := emailProviderRateLimitBackoffs
	emailProviderRateLimitBackoffs = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	defer func() { emailProviderRateLimitBackoffs = orig }()

	attempts := 0
	address, err := createTempEmailWithRateLimitRetry(context.Background(), "TempMail.lol", 1, 1, func() (string, error) {
		attempts++
		if attempts <= 2 {
			return "", errors.New(`创建 TempMail.lol 邮箱 HTTP 429: {"error":"Rate limited (free)"}`)
		}
		return "ok@example.com", nil
	})
	if err != nil || address != "ok@example.com" {
		t.Fatalf("expected retry success, address=%q err=%v", address, err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 create attempts, got %d", attempts)
	}
}

func TestCreateTempEmailWithRateLimitRetryStopsAfterBackoffs(t *testing.T) {
	orig := emailProviderRateLimitBackoffs
	emailProviderRateLimitBackoffs = []time.Duration{time.Millisecond, time.Millisecond}
	defer func() { emailProviderRateLimitBackoffs = orig }()

	attempts := 0
	_, err := createTempEmailWithRateLimitRetry(context.Background(), "TempMail.lol", 1, 1, func() (string, error) {
		attempts++
		return "", errors.New(`创建 TempMail.lol 邮箱 HTTP 429: {"error":"Rate limited (free)"}`)
	})
	if err == nil || !isEmailProviderRateLimitError(err.Error()) {
		t.Fatalf("expected final rate-limit error, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected initial attempt plus 2 retries, got %d", attempts)
	}
}

func TestTempMailLOLCreate403CountryBlockedIsProviderAccessBlocked(t *testing.T) {
	errText := `创建 TempMail.lol 邮箱 HTTP 403: {"error":"The country you are requesting from (TR) is not allowed to use the API free tier due to consistent API abuse."}`
	if !isEmailProviderAccessBlockedError(errText) {
		t.Fatalf("TempMail.lol country-blocked HTTP 403 should be treated as provider access block")
	}
	if isEmailProviderRateLimitError(errText) {
		t.Fatalf("country-blocked HTTP 403 should not be treated as retryable rate limit")
	}
}

func TestOrdinary403IsNotProviderAccessBlocked(t *testing.T) {
	errText := "创建 TempMail.lol 邮箱 HTTP 403: forbidden"
	if isEmailProviderAccessBlockedError(errText) {
		t.Fatalf("generic 403 without country/free-tier text should not be treated as provider access block")
	}
}

func TestEmailProxyUsesClashWhenSameEndpoint(t *testing.T) {
	if !emailProxyUsesClash("socks5://127.0.0.1:7890", "http://127.0.0.1:7890") {
		t.Fatalf("same local endpoint should be treated as email proxy reusing Clash")
	}
}

func TestEmailProxyUsesClashRequiresEmailProxy(t *testing.T) {
	if emailProxyUsesClash("", "http://127.0.0.1:7890") {
		t.Fatalf("blank email proxy is direct and should not be treated as Clash")
	}
}

func TestEmailProxyUsesClashRejectsDifferentEndpoint(t *testing.T) {
	if emailProxyUsesClash("socks5://127.0.0.1:7891", "http://127.0.0.1:7890") {
		t.Fatalf("different endpoint should not be treated as reusing Clash")
	}
}
