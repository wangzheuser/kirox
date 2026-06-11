package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	httputil "reg_go/internal/http"
)

// Info 代理检测结果
type Info struct {
	OK             bool     `json:"ok"`
	Scheme         string   `json:"scheme"`
	IP             string   `json:"ip"`
	Country        string   `json:"country"`
	Region         string   `json:"region"`
	City           string   `json:"city"`
	ISP            string   `json:"isp"`
	Error          string   `json:"error,omitempty"`
	Templated      bool     `json:"templated,omitempty"`
	Pool           bool     `json:"pool,omitempty"`
	Attempts       int      `json:"attempts,omitempty"`
	SuccessAttempt int      `json:"successAttempt,omitempty"`
	Target         string   `json:"target,omitempty"`
	DurationMs     int64    `json:"durationMs,omitempty"`
	Errors         []string `json:"errors,omitempty"`
	Clash          bool     `json:"clash,omitempty"`
	ClashVersion   string   `json:"clashVersion,omitempty"`
	ClashGroup     string   `json:"clashGroup,omitempty"`
	ClashNode      string   `json:"clashNode,omitempty"`
	ClashDelayMs   int      `json:"clashDelayMs,omitempty"`
	ClashSkipped   bool     `json:"clashSkipped,omitempty"`
}

// Detect 通过给定代理访问 IP 查询接口，返回出口 IP 和归属信息。
func Detect(proxyURL string) Info {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return Info{Error: "代理为空"}
	}
	templated := HasURLTemplate(proxyURL)
	if templated {
		return detectTemplatedProxy(proxyURL)
	}

	runtimeProxyURL := RenderURLTemplate(proxyURL)

	scheme := schemeOf(runtimeProxyURL, proxyURL)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	result := make(chan Info, 1)
	go func() {
		client, err := httputil.NewTLSClientWithTimeout(runtimeProxyURL, true, 8)
		if err != nil {
			result <- Info{Scheme: scheme, Error: simplifyProxyErr(err.Error()), Templated: templated}
			return
		}
		req, _ := fhttp.NewRequest("GET", "http://ip-api.com/json/?lang=zh-CN&fields=status,message,country,regionName,city,isp,query", nil)
		req.Header.Set("User-Agent", "kirox/proxy-check")
		resp, err := client.Do(req)
		if err != nil {
			result <- Info{Scheme: scheme, Error: simplifyProxyErr(err.Error()), Templated: templated}
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			result <- Info{Scheme: scheme, Error: fmt.Sprintf("HTTP %d", resp.StatusCode), Templated: templated}
			return
		}
		var data struct {
			Status, Message, Country, RegionName, City, ISP, Query string
		}
		if err := json.Unmarshal(body, &data); err != nil {
			result <- Info{Scheme: scheme, Error: "解析响应失败", Templated: templated}
			return
		}
		if data.Status != "success" {
			msg := data.Message
			if msg == "" {
				msg = "查询失败"
			}
			result <- Info{Scheme: scheme, Error: msg, Templated: templated}
			return
		}
		result <- Info{
			OK:        true,
			Scheme:    scheme,
			IP:        data.Query,
			Country:   data.Country,
			Region:    data.RegionName,
			City:      data.City,
			ISP:       data.ISP,
			Templated: templated,
		}
	}()

	select {
	case info := <-result:
		return info
	case <-ctx.Done():
		return Info{Scheme: scheme, Error: "检测超时", Templated: templated}
	}
}

// detectTemplatedProxy 检测模板代理，并确保设置页调用在固定时间内返回。
func detectTemplatedProxy(proxyURL string) Info {
	return detectTemplatedProxyWithOptions(proxyURL, DefaultDetectSelectOptions())
}

// detectTemplatedProxyWithOptions 使用指定选项检测模板代理，便于单元测试注入阻塞场景。
func detectTemplatedProxyWithOptions(proxyURL string, opts SelectOptions) Info {
	totalTimeout := opts.Timeout * time.Duration(opts.MaxAttempts)
	if totalTimeout <= 0 {
		totalTimeout = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), totalTimeout)
	defer cancel()

	start := time.Now()
	result := make(chan Info, 1)
	go func() {
		selection, err := SelectRuntimeProxy(ctx, proxyURL, opts)
		info := Info{
			Scheme:         schemeOf(selection.ProxyURL, proxyURL),
			Templated:      true,
			Pool:           true,
			Attempts:       selection.Attempts,
			SuccessAttempt: selection.SuccessAttempt,
			Target:         selection.TargetURL,
			DurationMs:     selection.Duration.Milliseconds(),
			Errors:         selection.Errors,
		}
		if err != nil {
			info.Error = simplifyProxyErr(err.Error())
			result <- info
			return
		}
		info.OK = true
		result <- info
	}()

	select {
	case info := <-result:
		return info
	case <-ctx.Done():
		return Info{
			Scheme:     schemeOf("", proxyURL),
			Error:      "检测超时",
			Templated:  true,
			Pool:       true,
			Attempts:   opts.MaxAttempts,
			Target:     opts.TargetURL,
			DurationMs: time.Since(start).Milliseconds(),
		}
	}
}

// DetectClash 切换 Clash 节点后，通过本地 Clash 代理检测出口。
func DetectClash(proxyURL string, config ClashConfig) Info {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return Info{Clash: true, Error: "代理为空"}
	}
	config = NormalizeClashConfig(config)
	client := NewClashClient(config)
	selection, err := client.SwitchToNextAvailable(context.Background())
	info := Info{
		Scheme:         schemeOf(proxyURL, proxyURL),
		Clash:          true,
		ClashVersion:   selection.Version,
		ClashGroup:     selection.ProxyGroup,
		ClashNode:      selection.Node,
		ClashDelayMs:   selection.DelayMs,
		ClashSkipped:   selection.SkippedTest,
		Attempts:       selection.Attempts,
		Target:         selection.TargetURL,
		DurationMs:     selection.DurationMs,
		Errors:         selection.Errors,
		SuccessAttempt: selection.Attempts,
	}
	if err != nil {
		info.Error = simplifyProxyErr(err.Error())
		return info
	}

	detected := Detect(proxyURL)
	detected.Clash = true
	detected.ClashVersion = selection.Version
	detected.ClashGroup = selection.ProxyGroup
	detected.ClashNode = selection.Node
	detected.ClashDelayMs = selection.DelayMs
	detected.ClashSkipped = selection.SkippedTest
	detected.Attempts = selection.Attempts
	detected.SuccessAttempt = selection.Attempts
	detected.Target = selection.TargetURL
	detected.DurationMs = selection.DurationMs
	detected.Errors = selection.Errors
	if !detected.OK && detected.Error != "" {
		detected.Error = "Clash 节点已切换，但代理检测失败: " + detected.Error
	}
	return detected
}

func schemeOf(proxyURL string, fallback string) string {
	if proxyURL == "" {
		proxyURL = fallback
	}
	if i := strings.Index(proxyURL, "://"); i > 0 {
		return strings.ToLower(proxyURL[:i])
	}
	return "http"
}

func simplifyProxyErr(s string) string {
	lower := strings.ToLower(s)
	switch {
	case strings.Contains(lower, "context deadline exceeded"),
		strings.Contains(lower, "client.timeout exceeded"),
		strings.Contains(lower, "operation timed out"),
		strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "timeout"),
		strings.Contains(lower, "deadline"):
		return "检测超时"
	case strings.Contains(s, "HTTPS 代理握手失败"):
		return "HTTPS 代理握手失败，请确认代理服务是否支持 HTTPS 代理协议"
	case strings.Contains(s, "代理 CONNECT 失败"):
		return s
	case strings.Contains(lower, "connection refused"):
		return "连接被拒绝"
	case strings.Contains(lower, "no such host"):
		return "域名解析失败"
	case strings.Contains(lower, "socks"):
		return "SOCKS 协商失败"
	case strings.Contains(lower, "proxy"):
		return "代理握手失败"
	}
	if len(s) > 80 {
		s = s[:80] + "..."
	}
	return s
}
