package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"reg_go/internal/proxy"
)

const runtimeProxyCountryEndpoint = "http://ip-api.com/json/?fields=status,message,countryCode"
const preferredRuntimeProxyCountryMaxAttempts = 40
const runtimeProxyCountryRiskCooldownAfter = 1
const runtimeProxyCountryRiskCooldown = 10 * time.Minute

type runtimeProxyCountryDetector func(context.Context, string) (string, error)

var preferredRuntimeProxyCountryCodes = map[string]struct{}{
	"KR": {},
	"JP": {},
}

var avoidedRuntimeProxyCountryCodes = map[string]struct{}{
	"US": {},
	"HK": {},
	"SG": {},
	"TW": {},
}

func detectRuntimeProxyCountryCode(ctx context.Context, proxyURL string) (string, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return "", fmt.Errorf("proxy is empty")
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(u),
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runtimeProxyCountryEndpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "kirox-proxy-locale/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("country endpoint HTTP %d", resp.StatusCode)
	}
	return parseRuntimeProxyCountryCode(body)
}

func parseRuntimeProxyCountryCode(body []byte) (string, error) {
	var data struct {
		Status      string `json:"status"`
		Message     string `json:"message"`
		CountryCode string `json:"countryCode"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	if strings.EqualFold(data.Status, "fail") {
		if data.Message != "" {
			return "", fmt.Errorf("%s", data.Message)
		}
		return "", fmt.Errorf("country endpoint returned fail")
	}
	code := strings.ToUpper(strings.TrimSpace(data.CountryCode))
	if code == "" {
		return "", fmt.Errorf("country endpoint returned empty countryCode")
	}
	return code, nil
}

func normalizeRuntimeProxyCountryCode(countryCode string) string {
	return strings.ToUpper(strings.TrimSpace(countryCode))
}

func isPreferredRuntimeProxyCountryCode(countryCode string) bool {
	_, ok := preferredRuntimeProxyCountryCodes[normalizeRuntimeProxyCountryCode(countryCode)]
	return ok
}

func isAvoidedRuntimeProxyCountryCode(countryCode string) bool {
	_, ok := avoidedRuntimeProxyCountryCodes[normalizeRuntimeProxyCountryCode(countryCode)]
	return ok
}

func hasRuntimeProxyCountryLocale(countryCode string) bool {
	_, ok := runtimeProxyBrowserLocaleForCountryCode(countryCode)
	return ok
}

func isAllowedRuntimeProxyFallbackCountryCode(countryCode string) bool {
	code := normalizeRuntimeProxyCountryCode(countryCode)
	if code == "" || isAvoidedRuntimeProxyCountryCode(code) {
		return false
	}
	return hasRuntimeProxyCountryLocale(code)
}

type runtimeProxyCountryRiskTracker struct {
	mu            sync.Mutex
	failures      map[string]int
	cooldownUntil map[string]time.Time
	cooldownAfter int
	cooldown      time.Duration
	now           func() time.Time
}

func newRuntimeProxyCountryRiskTracker() *runtimeProxyCountryRiskTracker {
	return &runtimeProxyCountryRiskTracker{
		failures:      make(map[string]int),
		cooldownUntil: make(map[string]time.Time),
		cooldownAfter: runtimeProxyCountryRiskCooldownAfter,
		cooldown:      runtimeProxyCountryRiskCooldown,
		now:           time.Now,
	}
}

func (t *runtimeProxyCountryRiskTracker) isCooling(countryCode string) bool {
	code := normalizeRuntimeProxyCountryCode(countryCode)
	if t == nil || code == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	until, ok := t.cooldownUntil[code]
	if !ok {
		return false
	}
	now := t.currentTimeLocked()
	if !now.Before(until) {
		delete(t.cooldownUntil, code)
		return false
	}
	return true
}

func (t *runtimeProxyCountryRiskTracker) recordRiskFailure(countryCode string) (string, int, bool, time.Time) {
	code := normalizeRuntimeProxyCountryCode(countryCode)
	if t == nil || code == "" {
		return "", 0, false, time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failures == nil {
		t.failures = make(map[string]int)
	}
	if t.cooldownUntil == nil {
		t.cooldownUntil = make(map[string]time.Time)
	}
	t.failures[code]++
	count := t.failures[code]
	if count < t.cooldownAfter {
		return code, count, false, time.Time{}
	}
	until := t.currentTimeLocked().Add(t.cooldown)
	t.cooldownUntil[code] = until
	return code, count, true, until
}

func (t *runtimeProxyCountryRiskTracker) recordSuccess(countryCode string) {
	code := normalizeRuntimeProxyCountryCode(countryCode)
	if t == nil || code == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, code)
	delete(t.cooldownUntil, code)
}

func (t *runtimeProxyCountryRiskTracker) currentTimeLocked() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

func selectRuntimeProxyWithCountryPreference(ctx context.Context, raw string, opts proxy.SelectOptions, detector runtimeProxyCountryDetector, maxPreferredAttempts int) (proxy.Selection, string, bool, error) {
	return selectRuntimeProxyWithCountryPolicy(ctx, raw, opts, detector, maxPreferredAttempts, nil, false)
}

func selectRuntimeProxyWithCountryPolicy(ctx context.Context, raw string, opts proxy.SelectOptions, detector runtimeProxyCountryDetector, maxPreferredAttempts int, riskTracker *runtimeProxyCountryRiskTracker, strictAvoidedFallback bool) (proxy.Selection, string, bool, error) {
	if !proxy.HasURLTemplate(raw) || detector == nil {
		selection, err := proxy.SelectRuntimeProxy(ctx, raw, opts)
		return selection, "", false, err
	}
	if maxPreferredAttempts <= 1 {
		selection, err := proxy.SelectRuntimeProxy(ctx, raw, opts)
		return selection, "", false, err
	}
	if maxPreferredAttempts < 1 {
		maxPreferredAttempts = 1
	}

	start := time.Now()
	singleAttemptOpts := opts
	singleAttemptOpts.MaxAttempts = 1

	var firstUsable proxy.Selection
	firstCountryCode := ""
	hasFirstUsable := false
	var firstNonAvoided proxy.Selection
	firstNonAvoidedCountryCode := ""
	hasFirstNonAvoided := false
	var errors []string
	for attempt := 1; attempt <= maxPreferredAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return proxy.Selection{
				Templated: true,
				Attempts:  attempt - 1,
				TargetURL: opts.TargetURL,
				Duration:  time.Since(start),
				Errors:    errors,
			}, "", false, err
		}

		selection, err := proxy.SelectRuntimeProxy(ctx, raw, singleAttemptOpts)
		if err != nil {
			errors = append(errors, fmt.Sprintf("第%d次代理不可用: %v", attempt, err))
			continue
		}

		countryCode, geoErr := detector(ctx, selection.ProxyURL)
		countryCode = normalizeRuntimeProxyCountryCode(countryCode)
		if geoErr != nil {
			errors = append(errors, fmt.Sprintf("第%d次地区探测失败: %v", attempt, geoErr))
		}
		cooling := geoErr == nil && riskTracker != nil && riskTracker.isCooling(countryCode)
		if cooling {
			errors = append(errors, fmt.Sprintf("第%d次出口地区 %s 风控冷却中，跳过优先使用", attempt, countryCode))
		}

		if !hasFirstUsable {
			firstUsable = selection
			firstUsable.SuccessAttempt = attempt
			firstCountryCode = countryCode
			hasFirstUsable = true
		}
		if geoErr == nil && !cooling && isAllowedRuntimeProxyFallbackCountryCode(countryCode) && !hasFirstNonAvoided {
			firstNonAvoided = selection
			firstNonAvoided.SuccessAttempt = attempt
			firstNonAvoidedCountryCode = countryCode
			hasFirstNonAvoided = true
		}

		if geoErr == nil && !cooling && isPreferredRuntimeProxyCountryCode(countryCode) {
			selection.Attempts = attempt
			selection.SuccessAttempt = attempt
			selection.Duration = time.Since(start)
			selection.Errors = errors
			return selection, countryCode, true, nil
		}
	}

	if hasFirstNonAvoided && (riskTracker == nil || !riskTracker.isCooling(firstNonAvoidedCountryCode)) {
		firstNonAvoided.Attempts = maxPreferredAttempts
		if firstNonAvoided.SuccessAttempt == 0 {
			firstNonAvoided.SuccessAttempt = 1
		}
		firstNonAvoided.Duration = time.Since(start)
		firstNonAvoided.Errors = errors
		return firstNonAvoided, firstNonAvoidedCountryCode, false, nil
	}
	if hasFirstNonAvoided {
		errors = append(errors, fmt.Sprintf("缓存候选出口地区 %s 已进入风控冷却，放弃回退", firstNonAvoidedCountryCode))
	}

	if hasFirstUsable {
		if riskTracker != nil && riskTracker.isCooling(firstCountryCode) {
			return proxy.Selection{
				Templated: true,
				Attempts:  maxPreferredAttempts,
				TargetURL: opts.TargetURL,
				Duration:  time.Since(start),
				Errors:    errors,
			}, "", false, fmt.Errorf("代理候选均为回避、冷却或无地区映射出口地区，已尝试 %d 次: %s", maxPreferredAttempts, strings.Join(errors, "；"))
		}
		if strictAvoidedFallback && !isAllowedRuntimeProxyFallbackCountryCode(firstCountryCode) {
			return proxy.Selection{
				Templated: true,
				Attempts:  maxPreferredAttempts,
				TargetURL: opts.TargetURL,
				Duration:  time.Since(start),
				Errors:    errors,
			}, "", false, fmt.Errorf("代理候选均为回避或无地区映射出口地区，已尝试 %d 次: %s", maxPreferredAttempts, strings.Join(errors, "；"))
		}
		firstUsable.Attempts = maxPreferredAttempts
		if firstUsable.SuccessAttempt == 0 {
			firstUsable.SuccessAttempt = 1
		}
		firstUsable.Duration = time.Since(start)
		firstUsable.Errors = errors
		return firstUsable, firstCountryCode, false, nil
	}

	return proxy.Selection{
		Templated: true,
		Attempts:  maxPreferredAttempts,
		TargetURL: opts.TargetURL,
		Duration:  time.Since(start),
		Errors:    errors,
	}, "", false, fmt.Errorf("代理候选均不可用或无法探测地区，已尝试 %d 次: %s", maxPreferredAttempts, strings.Join(errors, "；"))
}
