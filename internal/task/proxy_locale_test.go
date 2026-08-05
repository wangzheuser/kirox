package task

import (
	"context"
	"errors"
	"net/url"
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

func TestParseRuntimeProxyEgressInfoIncludesIPAndCountry(t *testing.T) {
	egress, err := parseRuntimeProxyEgressInfo([]byte(`{"status":"success","countryCode":"ro","query":"203.0.113.10","isp":"Example ISP","as":"AS64500 Example"}`))
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	if egress.IP != "203.0.113.10" || egress.CountryCode != "RO" || egress.ISP != "Example ISP" || egress.ASN != "AS64500 Example" {
		t.Fatalf("egress mismatch: %#v", egress)
	}
}

func TestSelectRuntimeProxyWithEgressPolicyAllowsUnmappedCountry(t *testing.T) {
	ids := []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
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
	detect := func(_ context.Context, _ string) (runtimeProxyEgressInfo, error) {
		return runtimeProxyEgressInfo{IP: "203.0.113.10", CountryCode: "ZZ"}, nil
	}

	selection, egress, preferred, err := selectRuntimeProxyWithEgressPolicy(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		1,
		runtimeProxyEgressSelectionPolicy{ExplorationPercent: 100},
	)
	if err != nil {
		t.Fatalf("selection returned error for unmapped country: %v", err)
	}
	if preferred {
		t.Fatalf("unmapped unknown egress should be exploratory, not historical preferred")
	}
	if egress.IP != "203.0.113.10" || egress.CountryCode != "ZZ" {
		t.Fatalf("egress mismatch: %#v", egress)
	}
	if selection.SuccessAttempt != 1 || !strings.Contains(selection.ProxyURL, ids[0]) {
		t.Fatalf("selected proxy mismatch: attempt=%d proxy=%q", selection.SuccessAttempt, selection.ProxyURL)
	}
}

func TestSelectRuntimeProxyWithEgressPolicyCoolsOnlyExactIP(t *testing.T) {
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
	detect := func(_ context.Context, proxyURL string) (runtimeProxyEgressInfo, error) {
		if strings.Contains(proxyURL, ids[0]) {
			return runtimeProxyEgressInfo{IP: "203.0.113.10", CountryCode: "RO"}, nil
		}
		return runtimeProxyEgressInfo{IP: "203.0.113.11", CountryCode: "RO"}, nil
	}

	selection, egress, _, err := selectRuntimeProxyWithEgressPolicy(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
		runtimeProxyEgressSelectionPolicy{
			ExplorationPercent: 100,
			IsCooling: func(egress runtimeProxyEgressInfo) bool {
				return egress.IP == "203.0.113.10"
			},
		},
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if egress.IP != "203.0.113.11" || egress.CountryCode != "RO" {
		t.Fatalf("same-country non-cooled IP should be selected, got %#v", egress)
	}
	if selection.SuccessAttempt != 2 || !strings.Contains(selection.ProxyURL, ids[1]) {
		t.Fatalf("cooled first IP should be skipped, attempt=%d proxy=%q", selection.SuccessAttempt, selection.ProxyURL)
	}
}

func TestRuntimeProxySourceKeyIsStableAndDoesNotExposeProxySecret(t *testing.T) {
	raw := " http://session-{uuid}:secret@127.0.0.1:9200 "
	keyA := runtimeProxySourceKey(raw)
	keyB := runtimeProxySourceKey(strings.TrimSpace(raw))
	if keyA == "" || keyA != keyB {
		t.Fatalf("source key should be stable, got %q and %q", keyA, keyB)
	}
	if strings.Contains(keyA, "secret") || strings.Contains(keyA, "127.0.0.1") || strings.Contains(keyA, "{uuid}") {
		t.Fatalf("source key should not expose raw proxy template, got %q", keyA)
	}
}

func TestRuntimeProxyEgressSelectionSamplesMultipleCandidates(t *testing.T) {
	if runtimeProxyEgressMaxAttempts != 10 {
		t.Fatalf("runtime proxy egress selection should sample 10 UUIDs, got %d", runtimeProxyEgressMaxAttempts)
	}
	if runtimeProxyEgressRiskCooldown != 10*time.Minute {
		t.Fatalf("runtime proxy egress risk cooldown should be 10 minutes, got %s", runtimeProxyEgressRiskCooldown)
	}
	if runtimeProxyEgressNetworkCooldown != 2*time.Minute {
		t.Fatalf("runtime proxy egress network cooldown should be 2 minutes, got %s", runtimeProxyEgressNetworkCooldown)
	}
}

func TestRuntimeProxyEgressSelectionCapsRepeatedErrors(t *testing.T) {
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string { return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
		Check: func(context.Context, string, string, time.Duration) error {
			return errors.New("proxy unavailable")
		},
	}

	selection, _, _, err := selectRuntimeProxyWithEgressPolicy(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		func(context.Context, string) (runtimeProxyEgressInfo, error) { return runtimeProxyEgressInfo{}, nil },
		runtimeProxyEgressMaxAttempts,
		runtimeProxyEgressSelectionPolicy{},
	)
	if err == nil {
		t.Fatalf("all unavailable proxies should fail")
	}
	if selection.Attempts != runtimeProxyEgressMaxAttempts || len(selection.Errors) != runtimeProxySelectionMaxErrors {
		t.Fatalf("attempts/errors = %d/%d, want %d/%d", selection.Attempts, len(selection.Errors), runtimeProxyEgressMaxAttempts, runtimeProxySelectionMaxErrors)
	}
	if strings.Contains(err.Error(), "第6次") {
		t.Fatalf("error should be capped: %v", err)
	}
}

func TestRuntimeProxyEgressHTTPClientDisablesIdleProxyConnections(t *testing.T) {
	proxyURL, err := url.Parse("http://127.0.0.1:9200")
	if err != nil {
		t.Fatalf("parse proxy url: %v", err)
	}
	client, transport := newRuntimeProxyEgressHTTPClient(proxyURL, 3*time.Second)
	if client.Timeout != 3*time.Second {
		t.Fatalf("client timeout = %s, want 3s", client.Timeout)
	}
	if !transport.DisableKeepAlives {
		t.Fatalf("egress detector should disable keep-alive for one-shot proxy probes")
	}
	if transport.MaxIdleConnsPerHost != -1 || transport.IdleConnTimeout != time.Second {
		t.Fatalf("unexpected idle connection limits: maxPerHost=%d idle=%s", transport.MaxIdleConnsPerHost, transport.IdleConnTimeout)
	}
}

func TestSelectRuntimeProxyWithEgressPolicyPrefersHistoricalSuccess(t *testing.T) {
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
	detect := func(_ context.Context, proxyURL string) (runtimeProxyEgressInfo, error) {
		if strings.Contains(proxyURL, ids[1]) {
			return runtimeProxyEgressInfo{IP: "203.0.113.20", CountryCode: "DE"}, nil
		}
		return runtimeProxyEgressInfo{IP: "203.0.113.10", CountryCode: "RO"}, nil
	}

	selection, egress, preferred, err := selectRuntimeProxyWithEgressPolicy(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		2,
		runtimeProxyEgressSelectionPolicy{
			ExplorationPercent: 0,
			HasSuccess: func(egress runtimeProxyEgressInfo) bool {
				return egress.IP == "203.0.113.20"
			},
		},
	)
	if err != nil {
		t.Fatalf("selection returned error: %v", err)
	}
	if !preferred || egress.IP != "203.0.113.20" {
		t.Fatalf("expected historical successful IP, preferred=%v egress=%#v", preferred, egress)
	}
	if selection.SuccessAttempt != 2 || !strings.Contains(selection.ProxyURL, ids[1]) {
		t.Fatalf("historical successful IP should be selected on second candidate, attempt=%d proxy=%q", selection.SuccessAttempt, selection.ProxyURL)
	}
}

func TestSelectRuntimeProxyWithEgressPolicyReturnsValidatedCandidateWhenGeoProbeFails(t *testing.T) {
	checks := 0
	detections := 0
	opts := proxy.SelectOptions{
		MaxAttempts: 1,
		Timeout:     time.Second,
		TargetURL:   "https://example.test/ping",
		UUIDFactory: func() string { return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" },
		Check: func(_ context.Context, _, _ string, _ time.Duration) error {
			checks++
			return nil
		},
	}
	detect := func(_ context.Context, _ string) (runtimeProxyEgressInfo, error) {
		detections++
		return runtimeProxyEgressInfo{}, errors.New("geo endpoint unavailable")
	}

	selection, egress, preferred, err := selectRuntimeProxyWithEgressPolicy(
		context.Background(),
		"http://session-{uuid}:secret@127.0.0.1:9200",
		opts,
		detect,
		runtimeProxyEgressMaxAttempts,
		runtimeProxyEgressSelectionPolicy{},
	)
	if err != nil {
		t.Fatalf("已验活候选不应因可选地理探测失败而被丢弃: %v", err)
	}
	if checks != 1 || detections != 1 {
		t.Fatalf("代理/地理探测次数 = %d/%d, want 1/1", checks, detections)
	}
	if selection.Attempts != 1 || selection.SuccessAttempt != 1 || selection.ProxyURL == "" {
		t.Fatalf("应立即返回首个已验活候选: %+v", selection)
	}
	if preferred || egress != (runtimeProxyEgressInfo{}) {
		t.Fatalf("地理探测失败不应标记历史优选: preferred=%v egress=%+v", preferred, egress)
	}
	if len(selection.Errors) != 1 || !strings.Contains(selection.Errors[0], "出口探测失败") {
		t.Fatalf("地理探测失败详情应保留: %v", selection.Errors)
	}
}
