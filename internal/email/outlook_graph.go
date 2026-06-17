package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"reg_go/internal/proxy"
)

var (
	outlookGraphTokenEndpoint = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	outlookGraphAPIBase       = "https://graph.microsoft.com/v1.0"
)

const outlookGraphScope = "offline_access Mail.ReadWrite Mail.Send User.Read"

type graphMessagesResponse struct {
	Value []graphMessage `json:"value"`
}

type graphMessage struct {
	Subject          string `json:"subject"`
	BodyPreview      string `json:"bodyPreview"`
	ReceivedDateTime string `json:"receivedDateTime"`
	Body             struct {
		Content string `json:"content"`
	} `json:"body"`
	ToRecipients []struct {
		EmailAddress struct {
			Address string `json:"address"`
		} `json:"emailAddress"`
	} `json:"toRecipients"`
}

type OutlookGraphProfile struct {
	PrimaryEmail       string
	Aliases            []string
	AliasDataAvailable bool
}

func (p OutlookGraphProfile) HasAddress(address string) bool {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(p.PrimaryEmail), address) {
		return true
	}
	for _, alias := range p.Aliases {
		if strings.EqualFold(strings.TrimSpace(alias), address) {
			return true
		}
	}
	return false
}

func (p OutlookGraphProfile) HasAliasData() bool {
	return p.AliasDataAvailable
}

// RefreshOutlookGraphTokenWithProxy 用 OutlookRegister 的 Graph scope 刷新 access_token。
func RefreshOutlookGraphTokenWithProxy(acc OutlookAccount, proxyURL string) (string, error) {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	form := url.Values{
		"client_id":     {acc.ClientID},
		"refresh_token": {acc.RefreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {outlookGraphScope},
	}

	client := httpClientWithProxy(runtimeProxyURL, emailRequestTimeout)
	resp, err := client.Post(outlookGraphTokenEndpoint, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("刷新失败 %d: %s", resp.StatusCode, string(body[:min(300, len(body))]))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}
	token, _ := result["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("响应中无 access_token")
	}
	return token, nil
}

func GetOutlookGraphUserPrincipalNameWithProxy(acc OutlookAccount, proxyURL string) (string, error) {
	profile, err := GetOutlookGraphProfileWithProxy(acc, proxyURL)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(profile.PrimaryEmail) == "" {
		return "", fmt.Errorf("Graph /me 响应中无 userPrincipalName")
	}
	return profile.PrimaryEmail, nil
}

func GetOutlookGraphProfileWithProxy(acc OutlookAccount, proxyURL string) (OutlookGraphProfile, error) {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	accessToken, err := RefreshOutlookGraphTokenWithProxy(acc, runtimeProxyURL)
	if err != nil {
		return OutlookGraphProfile{}, err
	}
	endpoint := strings.TrimRight(outlookGraphAPIBase, "/") + "/me?$select=userPrincipalName,mail,proxyAddresses,otherMails"
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return OutlookGraphProfile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := httpClientWithProxy(runtimeProxyURL, emailRequestTimeout).Do(req)
	if err != nil {
		return OutlookGraphProfile{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return OutlookGraphProfile{}, fmt.Errorf("Graph /me 查询失败 %d: %s", resp.StatusCode, string(body[:min(300, len(body))]))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return OutlookGraphProfile{}, fmt.Errorf("解析 Graph /me 响应失败: %w", err)
	}
	mail, _ := data["mail"].(string)
	upn, _ := data["userPrincipalName"].(string)
	primary := strings.TrimSpace(mail)
	if primary == "" {
		primary = strings.TrimSpace(upn)
	}
	if primary == "" {
		return OutlookGraphProfile{}, fmt.Errorf("Graph /me 响应中无 userPrincipalName")
	}
	aliases, aliasDataAvailable := collectGraphAliases(data, primary, upn)
	return OutlookGraphProfile{PrimaryEmail: primary, Aliases: aliases, AliasDataAvailable: aliasDataAvailable}, nil
}

func collectGraphAliases(data map[string]interface{}, values ...string) ([]string, bool) {
	seen := make(map[string]struct{})
	aliases := make([]string, 0)
	add := func(v string) {
		v = strings.TrimSpace(v)
		v = strings.TrimPrefix(v, "SMTP:")
		v = strings.TrimPrefix(v, "smtp:")
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		aliases = append(aliases, v)
	}
	for _, v := range values {
		add(v)
	}
	aliasDataAvailable := false
	for _, field := range []string{"proxyAddresses", "otherMails"} {
		if raw, exists := data[field]; exists {
			aliasDataAvailable = true
			arr, ok := raw.([]interface{})
			if !ok {
				continue
			}
			for _, raw := range arr {
				if v, ok := raw.(string); ok {
					add(v)
				}
			}
		}
	}
	return aliases, aliasDataAvailable
}

// WaitForOTPGraphWithProxy 通过 Microsoft Graph 轮询等待 AWS 验证码。
// 支持 context 取消，任务停止时立即中断轮询。
func WaitForOTPGraphWithProxy(ctx context.Context, acc OutlookAccount, after time.Time, timeout, interval int, proxyURL string) (string, error) {
	if interval <= 0 {
		interval = 5
	}
	if timeout <= 0 {
		timeout = 120
	}
	if after.IsZero() {
		// 缺少发送时间时仅回看很短窗口，避免误读历史验证码。
		after = time.Now().UTC().Add(-5 * time.Second)
	}

	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	targetEmail := strings.TrimSpace(acc.Email)
	if strings.TrimSpace(acc.RegistrationEmail) != "" {
		targetEmail = strings.TrimSpace(acc.RegistrationEmail)
	}
	log.Printf("[Outlook Graph] 等待验证码, 邮箱=%s, 起始时间=%s", targetEmail, after.Format(time.RFC3339))

	accessToken, err := refreshOutlookGraphTokenWithTransientRetry(ctx, acc, runtimeProxyURL)
	if err != nil {
		return "", fmt.Errorf("刷新 Outlook Graph Token 失败: %v", err)
	}

	codeRegex := regexp.MustCompile(`\b(\d{6})\b`)
	maxRetries := timeout / interval
	if maxRetries < 1 {
		maxRetries = 1
	}
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 每次轮询前检查 context 是否已取消
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		code, err := findGraphOTP(accessToken, targetEmail, after, codeRegex, runtimeProxyURL)
		if err != nil && attempt%5 == 0 {
			log.Printf("[Outlook Graph] 查询失败: %v, 重试中...", err)
		}
		if code != "" {
			log.Printf("[Outlook Graph] 获取到验证码: %s", code)
			return code, nil
		}
		if attempt%5 == 0 {
			log.Printf("[Outlook Graph] [%d/%d] 暂未找到新验证码...", attempt, maxRetries)
		}
		// 使用 select 等待，支持 context 取消时立即中断
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}

func refreshOutlookGraphTokenWithTransientRetry(ctx context.Context, acc OutlookAccount, proxyURL string) (string, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		token, err := RefreshOutlookGraphTokenWithProxy(acc, proxyURL)
		if err == nil {
			return token, nil
		}
		lastErr = err
		if !isTransientOutlookGraphTokenError(err) || attempt == maxAttempts {
			return "", err
		}
		wait := time.Duration(attempt) * time.Second
		log.Printf("[Outlook Graph] 刷新 Token 网络错误: %v；等待 %s 后重试 (%d/%d)", err, wait, attempt, maxAttempts)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(wait):
		}
	}
	return "", lastErr
}

func isTransientOutlookGraphTokenError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	lower := strings.ToLower(err.Error())
	transientMarkers := []string{
		"eof",
		"connection reset",
		"connection refused",
		"connection aborted",
		"tls handshake timeout",
		"timeout",
		"temporary failure",
		"server misbehaving",
	}
	for _, marker := range transientMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
func findGraphOTP(accessToken, targetEmail string, after time.Time, codeRegex *regexp.Regexp, proxyURL string) (string, error) {
	var lastErr error
	for _, folder := range []string{"inbox", "junkemail"} {
		messages, err := fetchGraphMessages(accessToken, folder, proxyURL)
		if err != nil {
			lastErr = err
			continue
		}
		for _, message := range messages {
			receivedAt, err := time.Parse(time.RFC3339, message.ReceivedDateTime)
			if err != nil || receivedAt.Before(after) {
				continue
			}
			// 校验邮件收件人是否为当前注册的别名邮箱。
			// 共享收件箱下并发注册时，需按收件人过滤，避免拿到其他别名的验证码。
			var toAddrs []string
			for _, r := range message.ToRecipients {
				toAddrs = append(toAddrs, r.EmailAddress.Address)
			}
			toField := strings.Join(toAddrs, ",")
			if toField == "" {
				// To 缺失时无法判别归属，保留旧行为继续尝试，但记录告警以便排查。
				log.Printf("[Outlook Graph] 警告: 邮件 Subject=%q 缺少 To 字段，无法校验收件人，按旧逻辑处理", message.Subject)
			} else if !recipientMatches(toField, targetEmail) {
				// 收件人不匹配当前别名，跳过该邮件，避免验证码错配。
				log.Printf("[Outlook Graph] 跳过非本别名邮件: Subject=%q, To=%v, 期望=%s", message.Subject, toAddrs, targetEmail)
				continue
			}

			text := strings.Join([]string{message.Subject, message.BodyPreview, message.Body.Content}, " ")
			if code := extractCodeFromText(text, codeRegex); code != "" {
				return code, nil
			}
		}
	}
	return "", lastErr
}

func fetchGraphMessages(accessToken, folder, proxyURL string) ([]graphMessage, error) {
	params := url.Values{}
	params.Set("$top", "10")
	params.Set("$orderby", "receivedDateTime desc")
	params.Set("$select", "subject,bodyPreview,body,receivedDateTime,toRecipients")
	endpoint := strings.TrimRight(outlookGraphAPIBase, "/") +
		"/me/mailFolders/" + url.PathEscape(folder) + "/messages?" + params.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Prefer", `outlook.body-content-type="text"`)

	resp, err := httpClientWithProxy(proxyURL, emailRequestTimeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Graph 查询失败 %d: %s", resp.StatusCode, string(body[:min(300, len(body))]))
	}

	var data graphMessagesResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 Graph 邮件响应失败: %w", err)
	}
	return data.Value, nil
}
