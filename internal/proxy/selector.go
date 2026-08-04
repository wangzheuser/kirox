package proxy

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/google/uuid"
	httputil "reg_go/internal/http"
)

const (
	DefaultProbeTarget       = "https://oidc.us-east-1.amazonaws.com/ping"
	defaultSelectAttempts    = 5
	defaultCandidateTimeout  = 15 * time.Second
	defaultDetectAttempts    = 3
	defaultDetectTimeout     = 2500 * time.Millisecond
	maxSelectorErrorMessages = 5
)

// SelectOptions 描述代理运行时节点选择参数。
type SelectOptions struct {
	MaxAttempts int
	Timeout     time.Duration
	TargetURL   string
	UUIDFactory func() string
	Check       func(context.Context, string, string, time.Duration) error
}

// Selection 描述一次代理运行时节点选择结果。
type Selection struct {
	ProxyURL       string
	MaskedProxyURL string
	Templated      bool
	Attempts       int
	SuccessAttempt int
	TargetURL      string
	Duration       time.Duration
	Errors         []string
}

// DefaultDetectSelectOptions 返回设置页检测使用的代理池抽样参数。
func DefaultDetectSelectOptions() SelectOptions {
	return SelectOptions{
		MaxAttempts: defaultDetectAttempts,
		Timeout:     defaultDetectTimeout,
		TargetURL:   DefaultProbeTarget,
	}
}

// DefaultRegisterSelectOptions 返回注册前选择运行时代理使用的抽样参数。
func DefaultRegisterSelectOptions() SelectOptions {
	return SelectOptions{
		MaxAttempts: defaultSelectAttempts,
		Timeout:     defaultCandidateTimeout,
		TargetURL:   DefaultProbeTarget,
	}
}

// SelectRuntimeProxy 从代理模板中选择一个当前可用的运行时代理。
func SelectRuntimeProxy(ctx context.Context, raw string, opts SelectOptions) (Selection, error) {
	start := time.Now()
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Selection{Duration: time.Since(start)}, nil
	}

	opts = normalizeSelectOptions(opts)
	templated := HasURLTemplate(raw)
	maxAttempts := 1
	if templated {
		maxAttempts = opts.MaxAttempts
	}

	var errs []string
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return Selection{
				Templated: templated,
				Attempts:  attempt - 1,
				TargetURL: opts.TargetURL,
				Duration:  time.Since(start),
				Errors:    errs,
			}, err
		}

		runtimeProxy := raw
		if templated {
			runtimeProxy = renderURLTemplate(raw, opts.UUIDFactory)
		}
		if err := opts.Check(ctx, runtimeProxy, opts.TargetURL, opts.Timeout); err != nil {
			errs = appendSelectorError(errs, fmt.Sprintf("第%d次: %s", attempt, sanitizeProxyError(err, runtimeProxy)))
			continue
		}

		return Selection{
			ProxyURL:       runtimeProxy,
			MaskedProxyURL: MaskURL(runtimeProxy),
			Templated:      templated,
			Attempts:       attempt,
			SuccessAttempt: attempt,
			TargetURL:      opts.TargetURL,
			Duration:       time.Since(start),
			Errors:         errs,
		}, nil
	}

	return Selection{
		Templated: templated,
		Attempts:  maxAttempts,
		TargetURL: opts.TargetURL,
		Duration:  time.Since(start),
		Errors:    errs,
	}, fmt.Errorf("代理候选均不可用，已尝试 %d 次: %s", maxAttempts, strings.Join(errs, "；"))
}

// CheckCandidate 验证单个运行时代理是否能访问目标注册端点。
func CheckCandidate(ctx context.Context, proxyURL, targetURL string, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if targetURL == "" {
		targetURL = DefaultProbeTarget
	}
	if timeout <= 0 {
		timeout = defaultCandidateTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := httputil.NewOneShotTLSClientWithTimeout(proxyURL, true, timeoutSeconds(timeout))
	if err != nil {
		return err
	}
	defer client.CloseIdleConnections()

	req, err := fhttp.NewRequest("GET", targetURL, nil)
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Accept", "application/json,*/*")
	req.Header.Set("User-Agent", "kirox/proxy-pool-check")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Errorf("探测端点 HTTP %d", resp.StatusCode)
	}
	return nil
}

// MaskURL 返回适合日志/UI 展示的代理地址，避免泄漏 UUID 和密码。
func MaskURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(maskParseableProxyURL(raw))
	if err != nil || u.Host == "" {
		return "<proxy>"
	}
	path := u.EscapedPath()
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	if u.User != nil {
		username := maskProxyUsername(u.User.Username())
		if username == "" {
			username = "<user>"
		}
		return fmt.Sprintf("%s://%s:***@%s%s", u.Scheme, username, u.Host, path)
	}
	return fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, path)
}

// maskParseableProxyURL 将模板占位符替换为合法虚构值，仅用于脱敏前解析。
func maskParseableProxyURL(raw string) string {
	if !strings.Contains(raw, uuidPlaceholder) {
		return raw
	}

	// 原始 {uuid} 放在 userinfo 中不是合法 URL 字符，解析前用固定假值占位。
	return strings.ReplaceAll(raw, uuidPlaceholder, "00000000000000000000000000000000")
}

func normalizeSelectOptions(opts SelectOptions) SelectOptions {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = defaultSelectAttempts
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultCandidateTimeout
	}
	if strings.TrimSpace(opts.TargetURL) == "" {
		opts.TargetURL = DefaultProbeTarget
	}
	if opts.UUIDFactory == nil {
		opts.UUIDFactory = uuid.NewString
	}
	if opts.Check == nil {
		opts.Check = CheckCandidate
	}
	return opts
}

func maskProxyUsername(username string) string {
	if username == "" {
		return ""
	}
	if strings.HasPrefix(username, "node.") {
		return "node.<uuid>"
	}
	return "<user>"
}

// SanitizeError 返回适合日志/UI 展示的代理错误，避免错误链中泄漏代理账号、密码或运行时 UUID。
func SanitizeError(err error, proxyURL string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if proxyURL != "" {
		msg = strings.ReplaceAll(msg, proxyURL, MaskURL(proxyURL))
	}
	if len(msg) > 120 {
		msg = msg[:120] + "..."
	}
	return msg
}

func sanitizeProxyError(err error, proxyURL string) string {
	return SanitizeError(err, proxyURL)
}

func appendSelectorError(errs []string, msg string) []string {
	if len(errs) >= maxSelectorErrorMessages {
		return errs
	}
	return append(errs, msg)
}

func timeoutSeconds(timeout time.Duration) int {
	if timeout <= 0 {
		timeout = defaultCandidateTimeout
	}
	seconds := int(timeout / time.Second)
	if timeout%time.Second != 0 {
		seconds++
	}
	if seconds <= 0 {
		seconds = 1
	}
	return seconds
}
