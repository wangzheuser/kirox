package http

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"sort"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const (
	DefaultUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"
	DefaultSecUA = `"Chromium";v="137", "Not/A)Brand";v="24", "Google Chrome";v="137"`
)

// Hex4 生成 4 位随机十六进制
func Hex4() string {
	const chars = "0123456789abcdef"
	b := make([]byte, 4)
	for i := range b {
		b[i] = chars[rand.Intn(16)]
	}
	return string(b)
}

// Awsccc 生成 awsccc cookie 值
func Awsccc() string {
	d := map[string]interface{}{
		"e": 1, "p": 1, "f": 1, "a": 1,
		"i": fmt.Sprintf("%s-%s-4%s-%s-%s%s%s",
			Hex4()+Hex4(), Hex4(), Hex4()[1:], Hex4(), Hex4(), Hex4(), Hex4()),
		"v": "1",
	}
	b, _ := json.Marshal(d)
	return base64.StdEncoding.EncodeToString(b)
}

// UbidGen 生成 ubid cookie 值
func UbidGen() string {
	d7 := make([]byte, 7)
	d6 := make([]byte, 6)
	for i := range d7 {
		d7[i] = byte('0' + rand.Intn(10))
	}
	for i := range d6 {
		d6[i] = byte('0' + rand.Intn(10))
	}
	return fmt.Sprintf("186-%s-%s", string(d7), string(d6))
}

// VisitorID 生成随机 visitor ID
func VisitorID() string {
	return fmt.Sprintf("%s%s-%s-7%s-%s-%s%s%s",
		Hex4(), Hex4(), Hex4(), Hex4()[1:], Hex4(), Hex4(), Hex4(), Hex4())
}

// PKCE 生成 PKCE code_verifier 和 code_challenge
func PKCE() (verifier, challenge string) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(rand.Intn(256))
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return
}

func chromeProfileVersion(chromeVer ...string) string {
	if len(chromeVer) == 0 {
		return ""
	}
	return chromeVer[0]
}

func chromeProfileKeyFromVersion(version string) string {
	version = strings.TrimSpace(version)
	major := version
	if dot := strings.IndexByte(major, '.'); dot >= 0 {
		major = major[:dot]
	}
	switch major {
	case "120", "124", "131", "133", "144":
		return "chrome_" + major
	default:
		return "chrome_144"
	}
}

func chromeClientProfile(version string) profiles.ClientProfile {
	switch chromeProfileKeyFromVersion(version) {
	case "chrome_120":
		return profiles.Chrome_120
	case "chrome_124":
		return profiles.Chrome_124
	case "chrome_131":
		return profiles.Chrome_131
	case "chrome_133":
		return profiles.Chrome_133
	default:
		return profiles.Chrome_144
	}
}

var newTLSClientWithTimeout = NewTLSClientWithTimeout

func defaultTLSClientTransportOptions() *tls_client.TransportOptions {
	idleTimeout := 10 * time.Second
	return &tls_client.TransportOptions{
		IdleConnTimeout:     &idleTimeout,
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 4,
	}
}

func oneShotTLSClientTransportOptions() *tls_client.TransportOptions {
	idleTimeout := time.Second
	return &tls_client.TransportOptions{
		IdleConnTimeout:     &idleTimeout,
		MaxIdleConns:        0,
		MaxIdleConnsPerHost: -1,
		DisableKeepAlives:   true,
	}
}

// NewTLSClient 创建带 TLS 指纹伪装的 HTTP 客户端。
func NewTLSClient(proxy string, followRedirect bool, chromeVer ...string) tls_client.HttpClient {
	client, err := newTLSClientWithTimeout(proxy, followRedirect, 60, chromeVer...)
	if err != nil {
		panic(fmt.Sprintf("创建 TLS 客户端失败: %v", err))
	}
	return client
}

// NewTLSClientWithTimeout 创建带 TLS 指纹伪装和自定义超时的 HTTP 客户端。
func NewTLSClientWithTimeout(proxy string, followRedirect bool, timeoutSeconds int, chromeVer ...string) (tls_client.HttpClient, error) {
	return newTLSClientWithTransportOptions(proxy, followRedirect, timeoutSeconds, defaultTLSClientTransportOptions(), chromeVer...)
}

// NewOneShotTLSClientWithTimeout 创建短生命周期 TLS 客户端。
// 用于代理候选探测等单请求场景，禁用 keep-alive，避免大量短任务占住本地代理连接。
func NewOneShotTLSClientWithTimeout(proxy string, followRedirect bool, timeoutSeconds int, chromeVer ...string) (tls_client.HttpClient, error) {
	return newTLSClientWithTransportOptions(proxy, followRedirect, timeoutSeconds, oneShotTLSClientTransportOptions(), chromeVer...)
}

func newTLSClientWithTransportOptions(proxy string, followRedirect bool, timeoutSeconds int, transportOptions *tls_client.TransportOptions, chromeVer ...string) (tls_client.HttpClient, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	effectiveProxy := proxy
	if isHTTPSProxyURL(proxy) {
		bridgeURL, err := bridgeHTTPSProxyURL(proxy, time.Duration(timeoutSeconds)*time.Second)
		if err != nil {
			return nil, fmt.Errorf("启动 HTTPS 代理桥失败: %w", err)
		}
		effectiveProxy = bridgeURL
	}
	profile := chromeClientProfile(chromeProfileVersion(chromeVer...))
	opts := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(timeoutSeconds),
		tls_client.WithClientProfile(profile),
		tls_client.WithInsecureSkipVerify(),
	}
	if transportOptions != nil {
		opts = append(opts, tls_client.WithTransportOptions(transportOptions))
	}
	if !followRedirect {
		opts = append(opts, tls_client.WithNotFollowRedirects())
	}
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), opts...)
	if err != nil {
		return nil, fmt.Errorf("创建 TLS 客户端失败: %w", err)
	}
	if effectiveProxy != "" {
		if err := client.SetProxy(effectiveProxy); err != nil {
			return nil, fmt.Errorf("设置代理失败: %w", err)
		}
	}
	return client, nil
}

// NewNoRedirectTLSClient 创建不跟随重定向的 TLS 客户端
func NewNoRedirectTLSClient(proxy string, chromeVer ...string) tls_client.HttpClient {
	return NewTLSClient(proxy, false, chromeVer...)
}

// ExtractParam 从 URL 中提取查询参数
func ExtractParam(rawURL, key string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get(key)
}

// SplitAfter 从字符串中提取分隔符后的内容
func SplitAfter(s, sep string) string {
	idx := strings.Index(s, sep)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(sep):]
	if i := strings.IndexByte(rest, '&'); i >= 0 {
		return rest[:i]
	}
	return rest
}

// GetNestedMap 获取嵌套 map
func GetNestedMap(data map[string]interface{}, keys ...string) map[string]interface{} {
	current := data
	for _, k := range keys {
		next, ok := current[k].(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// GetNestedStringMap 获取嵌套的 string map
func GetNestedStringMap(data map[string]interface{}, key string) map[string]string {
	if data == nil {
		return nil
	}
	nested, ok := data[key].(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string]string)
	for k, v := range nested {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// SetHeaders 设置请求头，并使用稳定的 Chrome 风格顺序。
func SetHeaders(req *fhttp.Request, headers map[string]string) {
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	preferred := []string{
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"upgrade-insecure-requests",
		"user-agent",
		"accept",
		"content-type",
		"origin",
		"sec-fetch-site",
		"sec-fetch-mode",
		"sec-fetch-user",
		"sec-fetch-dest",
		"referer",
		"accept-encoding",
		"accept-language",
		"cookie",
		"x-amzn-requestid",
		"x-amz-date",
		"x-amz-sso_bearer_token",
		"x-amz-sso-bearer-token",
		"priority",
	}
	present := make(map[string]bool, len(headers))
	for k := range headers {
		present[strings.ToLower(k)] = true
	}
	order := make([]string, 0, len(headers))
	seen := make(map[string]bool, len(headers))
	for _, key := range preferred {
		if present[key] {
			order = append(order, key)
			seen[key] = true
		}
	}
	var rest []string
	for key := range present {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	order = append(order, rest...)
	req.Header[fhttp.HeaderOrderKey] = order
}

// SaveCookies 从 Set-Cookie 头中提取并保存 cookies
func SaveCookies(cookies map[string]string, headers map[string][]string) {
	skip := map[string]bool{
		"path": true, "domain": true, "expires": true,
		"max-age": true, "secure": true, "httponly": true, "samesite": true,
	}
	for _, vals := range headers {
		for _, raw := range vals {
			if !strings.Contains(raw, "=") {
				continue
			}
			kv := strings.SplitN(strings.Split(raw, ";")[0], "=", 2)
			if len(kv) == 2 {
				k := strings.TrimSpace(kv[0])
				v := strings.TrimSpace(kv[1])
				if !skip[strings.ToLower(k)] && k != "" {
					cookies[k] = v
				}
			}
		}
	}
}
