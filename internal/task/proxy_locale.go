package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reg_go/internal/proxy"
	"reg_go/internal/storage"
)

const runtimeProxyEgressEndpoint = "http://ip-api.com/json/?fields=status,message,countryCode,query,isp,as"
const runtimeProxyEgressMaxAttempts = 10
const runtimeProxyEgressRiskCooldown = 10 * time.Minute
const runtimeProxySelectionMaxErrors = 5

type runtimeProxyEgressDetector func(context.Context, string) (runtimeProxyEgressInfo, error)

type runtimeProxyEgressInfo struct {
	IP          string
	CountryCode string
	ISP         string
	ASN         string
}

type runtimeProxyEgressSelectionPolicy struct {
	ExplorationPercent int
	IsCooling          func(runtimeProxyEgressInfo) bool
	HasSuccess         func(runtimeProxyEgressInfo) bool
}

func detectRuntimeProxyEgressInfo(ctx context.Context, proxyURL string) (runtimeProxyEgressInfo, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return runtimeProxyEgressInfo{}, fmt.Errorf("proxy is empty")
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return runtimeProxyEgressInfo{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	client, transport := newRuntimeProxyEgressHTTPClient(u, 8*time.Second)
	defer transport.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runtimeProxyEgressEndpoint, nil)
	if err != nil {
		return runtimeProxyEgressInfo{}, err
	}
	req.Header.Set("User-Agent", "kirox-proxy-locale/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return runtimeProxyEgressInfo{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return runtimeProxyEgressInfo{}, fmt.Errorf("egress endpoint HTTP %d", resp.StatusCode)
	}
	return parseRuntimeProxyEgressInfo(body)
}

func newRuntimeProxyEgressHTTPClient(proxyURL *url.URL, timeout time.Duration) (*http.Client, *http.Transport) {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		DialContext:           (&net.Dialer{Timeout: timeout, KeepAlive: -1}).DialContext,
		DisableKeepAlives:     true,
		MaxIdleConns:          0,
		MaxIdleConnsPerHost:   -1,
		IdleConnTimeout:       time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &http.Client{Timeout: timeout, Transport: transport}, transport
}

func parseRuntimeProxyCountryCode(body []byte) (string, error) {
	egress, err := parseRuntimeProxyEgressPayload(body, false)
	if err != nil {
		return "", err
	}
	if egress.CountryCode == "" {
		return "", fmt.Errorf("country endpoint returned empty countryCode")
	}
	return egress.CountryCode, nil
}

func parseRuntimeProxyEgressInfo(body []byte) (runtimeProxyEgressInfo, error) {
	return parseRuntimeProxyEgressPayload(body, true)
}

func parseRuntimeProxyEgressPayload(body []byte, requireIP bool) (runtimeProxyEgressInfo, error) {
	var data struct {
		Status      string `json:"status"`
		Message     string `json:"message"`
		CountryCode string `json:"countryCode"`
		Query       string `json:"query"`
		ISP         string `json:"isp"`
		ASN         string `json:"as"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return runtimeProxyEgressInfo{}, err
	}
	if strings.EqualFold(data.Status, "fail") {
		if data.Message != "" {
			return runtimeProxyEgressInfo{}, fmt.Errorf("%s", data.Message)
		}
		return runtimeProxyEgressInfo{}, fmt.Errorf("egress endpoint returned fail")
	}
	egress := runtimeProxyEgressInfo{
		IP:          strings.TrimSpace(data.Query),
		CountryCode: normalizeRuntimeProxyCountryCode(data.CountryCode),
		ISP:         strings.TrimSpace(data.ISP),
		ASN:         strings.TrimSpace(data.ASN),
	}
	if requireIP && egress.IP == "" {
		return runtimeProxyEgressInfo{}, fmt.Errorf("egress endpoint returned empty query")
	}
	return egress, nil
}

func normalizeRuntimeProxyCountryCode(countryCode string) string {
	return strings.ToUpper(strings.TrimSpace(countryCode))
}

func selectRuntimeProxyWithEgressPolicy(ctx context.Context, raw string, opts proxy.SelectOptions, detector runtimeProxyEgressDetector, maxPreferredAttempts int, policy runtimeProxyEgressSelectionPolicy) (proxy.Selection, runtimeProxyEgressInfo, bool, error) {
	if !proxy.HasURLTemplate(raw) || detector == nil {
		selection, err := proxy.SelectRuntimeProxy(ctx, raw, opts)
		return selection, runtimeProxyEgressInfo{}, false, err
	}
	if maxPreferredAttempts < 1 {
		maxPreferredAttempts = 1
	}

	start := time.Now()
	singleAttemptOpts := opts
	singleAttemptOpts.MaxAttempts = 1

	var firstNonCooling proxy.Selection
	var firstNonCoolingEgress runtimeProxyEgressInfo
	hasFirstNonCooling := false
	var errors []string
	for attempt := 1; attempt <= maxPreferredAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return proxy.Selection{
				Templated: true,
				Attempts:  attempt - 1,
				TargetURL: opts.TargetURL,
				Duration:  time.Since(start),
				Errors:    errors,
			}, runtimeProxyEgressInfo{}, false, err
		}

		selection, err := proxy.SelectRuntimeProxy(ctx, raw, singleAttemptOpts)
		if err != nil {
			errors = appendRuntimeProxySelectionError(errors, fmt.Sprintf("第%d次代理不可用: %v", attempt, err))
			continue
		}

		egress, geoErr := detector(ctx, selection.ProxyURL)
		egress = normalizeRuntimeProxyEgressInfo(egress)
		if geoErr != nil {
			errors = appendRuntimeProxySelectionError(errors, fmt.Sprintf("第%d次出口探测失败: %v", attempt, geoErr))
			selection.Attempts = attempt
			selection.SuccessAttempt = attempt
			selection.Duration = time.Since(start)
			selection.Errors = errors
			return selection, egress, false, nil
		}

		cooling := policy.IsCooling != nil && policy.IsCooling(egress)
		if cooling {
			errors = appendRuntimeProxySelectionError(errors, fmt.Sprintf("第%d次出口 IP %s 风控冷却中，跳过", attempt, egress.IP))
			continue
		}

		if !hasFirstNonCooling {
			firstNonCooling = selection
			firstNonCooling.SuccessAttempt = attempt
			firstNonCoolingEgress = egress
			hasFirstNonCooling = true
		}

		preferred := policy.HasSuccess != nil && policy.HasSuccess(egress)
		if preferred || shouldExploreRuntimeProxyEgress(policy.ExplorationPercent) {
			selection.Attempts = attempt
			selection.SuccessAttempt = attempt
			selection.Duration = time.Since(start)
			selection.Errors = errors
			return selection, egress, preferred, nil
		}
	}

	if hasFirstNonCooling {
		firstNonCooling.Attempts = maxPreferredAttempts
		if firstNonCooling.SuccessAttempt == 0 {
			firstNonCooling.SuccessAttempt = 1
		}
		firstNonCooling.Duration = time.Since(start)
		firstNonCooling.Errors = errors
		return firstNonCooling, firstNonCoolingEgress, false, nil
	}

	return proxy.Selection{
		Templated: true,
		Attempts:  maxPreferredAttempts,
		TargetURL: opts.TargetURL,
		Duration:  time.Since(start),
		Errors:    errors,
	}, runtimeProxyEgressInfo{}, false, fmt.Errorf("代理候选均不可用或出口 IP 均在冷却中，已尝试 %d 次: %s", maxPreferredAttempts, strings.Join(errors, "；"))
}

func appendRuntimeProxySelectionError(errors []string, message string) []string {
	if len(errors) >= runtimeProxySelectionMaxErrors {
		return errors
	}
	return append(errors, message)
}

func normalizeRuntimeProxyEgressInfo(egress runtimeProxyEgressInfo) runtimeProxyEgressInfo {
	egress.IP = strings.TrimSpace(egress.IP)
	egress.CountryCode = normalizeRuntimeProxyCountryCode(egress.CountryCode)
	egress.ISP = strings.TrimSpace(egress.ISP)
	egress.ASN = strings.TrimSpace(egress.ASN)
	return egress
}

func shouldExploreRuntimeProxyEgress(percent int) bool {
	percent = clampDomainExplorationPercent(percent)
	if percent <= 0 {
		return false
	}
	if percent >= 100 {
		return true
	}
	return rand.Intn(100) < percent
}

func runtimeProxySourceKey(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

func runtimeProxyEgressStorageIdentity(egress runtimeProxyEgressInfo) storage.ProxyEgressIdentity {
	egress = normalizeRuntimeProxyEgressInfo(egress)
	return storage.ProxyEgressIdentity{
		IP:          egress.IP,
		CountryCode: egress.CountryCode,
		ISP:         egress.ISP,
		ASN:         egress.ASN,
	}
}

func selectRuntimeProxyWithStoredEgressPolicy(ctx context.Context, raw string, opts proxy.SelectOptions, detector runtimeProxyEgressDetector, maxPreferredAttempts int, explorationPercent int) (proxy.Selection, runtimeProxyEgressInfo, bool, error) {
	sourceKey := runtimeProxySourceKey(raw)
	return selectRuntimeProxyWithEgressPolicy(ctx, raw, opts, detector, maxPreferredAttempts, runtimeProxyEgressSelectionPolicy{
		ExplorationPercent: explorationPercent,
		IsCooling: func(egress runtimeProxyEgressInfo) bool {
			return storage.IsProxyEgressCooling(sourceKey, egress.IP, time.Now())
		},
		HasSuccess: func(egress runtimeProxyEgressInfo) bool {
			return storage.HasProxyEgressSuccess(sourceKey, egress.IP)
		},
	})
}

func recordRuntimeProxyEgressAttempt(sourceKey string, egress runtimeProxyEgressInfo) error {
	return storage.RecordProxyEgressAttempt(sourceKey, runtimeProxyEgressStorageIdentity(egress))
}

func recordRuntimeProxyEgressSuccess(sourceKey string, egress runtimeProxyEgressInfo) error {
	return storage.RecordProxyEgressRegistrationSuccess(sourceKey, runtimeProxyEgressStorageIdentity(egress))
}

func recordRuntimeProxyEgressRiskFailure(sourceKey string, egress runtimeProxyEgressInfo) error {
	return storage.RecordProxyEgressRiskFailure(sourceKey, runtimeProxyEgressStorageIdentity(egress), runtimeProxyEgressRiskCooldown)
}

func recordRuntimeProxyEgressNetworkFailure(sourceKey string, egress runtimeProxyEgressInfo) error {
	return storage.RecordProxyEgressNetworkFailure(sourceKey, runtimeProxyEgressStorageIdentity(egress))
}
