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
	if preferredRuntimeProxyCountryMaxAttempts != 10 {
		t.Fatalf("runtime proxy selection should sample 10 UUIDs for registration country fit, got %d", preferredRuntimeProxyCountryMaxAttempts)
	}
	for _, country := range []string{"RO", "KR", "SG", "NL"} {
		if !isPreferredRuntimeProxyCountryCode(country) {
			t.Fatalf("runtime proxy country %s should be preferred for registration", country)
		}
	}
	for _, country := range []string{"US", "HK"} {
		if isPreferredRuntimeProxyCountryCode(country) {
			t.Fatalf("runtime proxy country %s should not be preferred after repeated TES blocks", country)
		}
		if !isAvoidedRuntimeProxyCountryCode(country) {
			t.Fatalf("runtime proxy country %s should be avoided after repeated TES blocks", country)
		}
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
