package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reg_go/internal/proxy"
)

const runtimeProxyCountryEndpoint = "http://ip-api.com/json/?fields=status,message,countryCode"
const preferredRuntimeProxyCountryMaxAttempts = 1

type runtimeProxyCountryDetector func(context.Context, string) (string, error)

var preferredRuntimeProxyCountryCodes = map[string]struct{}{
	"KR": {},
	"HK": {},
	"SG": {},
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

func selectRuntimeProxyWithCountryPreference(ctx context.Context, raw string, opts proxy.SelectOptions, detector runtimeProxyCountryDetector, maxPreferredAttempts int) (proxy.Selection, string, bool, error) {
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

		if !hasFirstUsable {
			firstUsable = selection
			firstCountryCode = countryCode
			hasFirstUsable = true
		}

		if geoErr == nil && isPreferredRuntimeProxyCountryCode(countryCode) {
			selection.Attempts = attempt
			selection.SuccessAttempt = attempt
			selection.Duration = time.Since(start)
			selection.Errors = errors
			return selection, countryCode, true, nil
		}
	}

	if hasFirstUsable {
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
