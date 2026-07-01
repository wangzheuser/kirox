package task

import (
	"context"
	"strings"
	"testing"
	"time"

	"reg_go/internal/proxy"
)

func TestParseRuntimeProxyCountryCode(t *testing.T) {
	code, err := parseRuntimeProxyCountryCode([]byte(`{"status":"success","countryCode":"ro"}`))
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	if code != "RO" {
		t.Fatalf("country code mismatch: got %q", code)
	}
}

func TestParseRuntimeProxyCountryCodeRejectsMissingCode(t *testing.T) {
	if _, err := parseRuntimeProxyCountryCode([]byte(`{"status":"success"}`)); err == nil {
		t.Fatalf("missing countryCode should be rejected")
	}
}

func TestRuntimeProxyCountryPreferenceSamplesMultipleCandidates(t *testing.T) {
	if preferredRuntimeProxyCountryMaxAttempts != 40 {
		t.Fatalf("runtime proxy selection should sample 40 UUIDs for registration country fit, got %d", preferredRuntimeProxyCountryMaxAttempts)
	}
	if runtimeProxyCountryRiskCooldownAfter != 1 {
		t.Fatalf("runtime proxy country risk cooldown should start after first hard risk failure, got %d", runtimeProxyCountryRiskCooldownAfter)
	}
	for _, country := range []string{"KR", "JP"} {
		if !isPreferredRuntimeProxyCountryCode(country) {
			t.Fatalf("runtime proxy country %s should be preferred for registration", country)
		}
	}
	for _, country := range []string{"RO", "SG", "NL", "DE"} {
		if isPreferredRuntimeProxyCountryCode(country) {
			t.Fatalf("runtime proxy country %s should be fallback-only until it has better probe evidence", country)
		}
	}
	for _, country := range []string{"US", "HK", "SG", "TW"} {
		if isPreferredRuntimeProxyCountryCode(country) {
			t.Fatalf("runtime proxy country %s should not be preferred after repeated TES blocks", country)
		}
		if !isAvoidedRuntimeProxyCountryCode(country) {
			t.Fatalf("runtime proxy country %s should be avoided after repeated TES blocks", country)
		}
	}
}

func TestSelectRuntimeProxyWithCountryPreferenceSkipsUnmappedFallbackCountry(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, _, _ string, _ time.Duration) error {
			return nil
		},
	}
	detect := func(_ context.Context, proxyURL string) (string, error) {
		if strings.Contains(proxyURL, ids[1]) {
			return "DE", nil
		}
		return "TW", nil
	}

	selection, countryCode, preferred, err := selectRuntimeProxyWithCountryPreference(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if preferred || countryCode != "DE" {
		t.Fatalf("expected mapped DE fallback after skipped unmapped TW, got preferred=%v country=%q", preferred, countryCode)
	}
	if selection.SuccessAttempt != 2 || !strings.Contains(selection.ProxyURL, ids[1]) {
		t.Fatalf("fallback should skip unmapped TW proxy and use second candidate, attempt=%d proxy=%q", selection.SuccessAttempt, selection.ProxyURL)
	}
}

func TestRuntimeProxyCountryRiskTrackerCoolsAfterThresholdAndResetsOnSuccess(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.Local)
	tracker := newRuntimeProxyCountryRiskTracker()
	tracker.now = func() time.Time { return now }
	tracker.cooldownAfter = 1
	tracker.cooldown = time.Minute

	if code, count, cooling, _ := tracker.recordRiskFailure(" ro "); code != "RO" || count != 1 || !cooling {
		t.Fatalf("first hard risk failure should cool immediately, got code=%q count=%d cooling=%v", code, count, cooling)
	}
	if !tracker.isCooling("ro") {
		t.Fatalf("country should be cooling after threshold")
	}

	tracker.recordSuccess("RO")
	if tracker.isCooling("RO") {
		t.Fatalf("success should clear country cooldown")
	}
}

func TestSelectRuntimeProxyWithCountryPreferenceDisabledUsesSelectorRetries(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	var checked []string
	opts := proxy.SelectOptions{
		MaxAttempts: 2,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, proxyURL, _ string, _ time.Duration) error {
			checked = append(checked, proxyURL)
			if strings.Contains(proxyURL, ids[0]) {
				return context.DeadlineExceeded
			}
			return nil
		},
	}
	detect := func(context.Context, string) (string, error) {
		t.Fatalf("country detector should not run when country preference is disabled")
		return "", nil
	}

	selection, countryCode, preferred, err := selectRuntimeProxyWithCountryPreference(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		1,
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if preferred || countryCode != "" {
		t.Fatalf("preference should be disabled, got preferred=%v country=%q", preferred, countryCode)
	}
	if selection.SuccessAttempt != 2 || !strings.Contains(selection.ProxyURL, ids[1]) {
		t.Fatalf("selector should retry to the second UUID, got attempt=%d proxy=%q", selection.SuccessAttempt, selection.ProxyURL)
	}
	if len(checked) != 2 {
		t.Fatalf("expected two selector checks, got %d", len(checked))
	}
}

func TestSelectRuntimeProxyWithCountryPreferenceKeepsTryingUntilPreferred(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	var checked []string
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, proxyURL, _ string, _ time.Duration) error {
			checked = append(checked, proxyURL)
			return nil
		},
	}
	detect := func(_ context.Context, proxyURL string) (string, error) {
		if strings.Contains(proxyURL, ids[1]) {
			return "KR", nil
		}
		return "US", nil
	}

	selection, countryCode, preferred, err := selectRuntimeProxyWithCountryPreference(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if !preferred || countryCode != "KR" {
		t.Fatalf("expected preferred KR proxy, got preferred=%v country=%q", preferred, countryCode)
	}
	if selection.Attempts != 2 || selection.SuccessAttempt != 2 {
		t.Fatalf("selection attempts = %d/%d, want 2/2", selection.SuccessAttempt, selection.Attempts)
	}
	if !strings.Contains(selection.ProxyURL, ids[1]) {
		t.Fatalf("selected proxy should use second UUID, got %q", selection.ProxyURL)
	}
	if len(checked) != 2 {
		t.Fatalf("expected two candidate checks, got %d", len(checked))
	}
}

func TestSelectRuntimeProxyWithCountryPreferenceFallsBackToFirstUsableProxy(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, _, _ string, _ time.Duration) error {
			return nil
		},
	}
	detect := func(_ context.Context, _ string) (string, error) {
		return "US", nil
	}

	selection, countryCode, preferred, err := selectRuntimeProxyWithCountryPreference(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if preferred || countryCode != "US" {
		t.Fatalf("expected non-preferred US fallback, got preferred=%v country=%q", preferred, countryCode)
	}
	if selection.Attempts != 2 || selection.SuccessAttempt != 1 {
		t.Fatalf("fallback attempts = %d/%d, want success attempt 1 after 2 probes", selection.SuccessAttempt, selection.Attempts)
	}
	if !strings.Contains(selection.ProxyURL, ids[0]) {
		t.Fatalf("fallback should use first usable proxy, got %q", selection.ProxyURL)
	}
}

func TestSelectRuntimeProxyWithCountryPreferenceFallsBackToFirstNonAvoidedProxy(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, _, _ string, _ time.Duration) error {
			return nil
		},
	}
	detect := func(_ context.Context, proxyURL string) (string, error) {
		if strings.Contains(proxyURL, ids[1]) {
			return "DE", nil
		}
		return "US", nil
	}

	selection, countryCode, preferred, err := selectRuntimeProxyWithCountryPreference(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if preferred || countryCode != "DE" {
		t.Fatalf("expected non-preferred but non-avoided DE fallback, got preferred=%v country=%q", preferred, countryCode)
	}
	if selection.Attempts != 2 || selection.SuccessAttempt != 2 {
		t.Fatalf("fallback attempts = %d/%d, want success attempt 2 after 2 probes", selection.SuccessAttempt, selection.Attempts)
	}
	if !strings.Contains(selection.ProxyURL, ids[1]) {
		t.Fatalf("fallback should skip avoided US proxy and use second candidate, got %q", selection.ProxyURL)
	}
}

func TestSelectRuntimeProxyWithCountryPreferenceSkipsAvoidedHongKongFallback(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, _, _ string, _ time.Duration) error {
			return nil
		},
	}
	detect := func(_ context.Context, proxyURL string) (string, error) {
		if strings.Contains(proxyURL, ids[1]) {
			return "DE", nil
		}
		return "HK", nil
	}

	selection, countryCode, preferred, err := selectRuntimeProxyWithCountryPreference(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if preferred || countryCode != "DE" {
		t.Fatalf("expected DE fallback after skipped HK, got preferred=%v country=%q", preferred, countryCode)
	}
	if selection.SuccessAttempt != 2 || !strings.Contains(selection.ProxyURL, ids[1]) {
		t.Fatalf("fallback should skip avoided HK proxy and use second candidate, attempt=%d proxy=%q", selection.SuccessAttempt, selection.ProxyURL)
	}
}

func TestSelectRuntimeProxyWithCountryPolicySkipsCoolingCountryWhenAlternativeExists(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, _, _ string, _ time.Duration) error {
			return nil
		},
	}
	detect := func(_ context.Context, proxyURL string) (string, error) {
		if strings.Contains(proxyURL, ids[0]) {
			return "RO", nil
		}
		return "JP", nil
	}
	tracker := newRuntimeProxyCountryRiskTracker()
	tracker.cooldownAfter = 1
	tracker.recordRiskFailure("RO")

	selection, countryCode, preferred, err := selectRuntimeProxyWithCountryPolicy(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
		tracker,
		false,
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if !preferred || countryCode != "JP" {
		t.Fatalf("expected preferred JP after RO cooled, got preferred=%v country=%q", preferred, countryCode)
	}
	if selection.SuccessAttempt != 2 || !strings.Contains(selection.ProxyURL, ids[1]) {
		t.Fatalf("cooled RO should be skipped in favor of second candidate, attempt=%d proxy=%q", selection.SuccessAttempt, selection.ProxyURL)
	}
}

func TestSelectRuntimeProxyWithCountryPolicyDoesNotReturnStaleFallbackAfterCountryCools(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, _, _ string, _ time.Duration) error {
			return nil
		},
	}
	tracker := newRuntimeProxyCountryRiskTracker()
	tracker.cooldownAfter = 1
	detect := func(_ context.Context, proxyURL string) (string, error) {
		if strings.Contains(proxyURL, ids[0]) {
			return "RO", nil
		}
		tracker.recordRiskFailure("RO")
		return "US", nil
	}

	_, countryCode, _, err := selectRuntimeProxyWithCountryPolicy(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
		tracker,
		true,
	)
	if err == nil {
		t.Fatalf("stale fallback country %q should not be returned after it enters cooldown", countryCode)
	}
	if strings.EqualFold(countryCode, "RO") {
		t.Fatalf("cooled stale fallback RO must not be returned")
	}
}

func TestSelectRuntimeProxyWithCountryPolicyStrictAvoidsOnlyAvoidedCountries(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	var idx int
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string {
			id := ids[idx]
			idx++
			return id
		},
		Check: func(_ context.Context, _, _ string, _ time.Duration) error {
			return nil
		},
	}
	detect := func(_ context.Context, proxyURL string) (string, error) {
		if strings.Contains(proxyURL, ids[0]) {
			return "US", nil
		}
		return "HK", nil
	}

	_, _, _, err := selectRuntimeProxyWithCountryPolicy(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
		nil,
		true,
	)
	if err == nil || !strings.Contains(err.Error(), "回避或无地区映射出口地区") {
		t.Fatalf("strict mode should reject all avoided countries, got %v", err)
	}
}
