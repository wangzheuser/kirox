package email

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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

// WaitForOTPGraphWithProxy 通过 Microsoft Graph 轮询等待 AWS 验证码。
func WaitForOTPGraphWithProxy(acc OutlookAccount, after time.Time, timeout, interval int, proxyURL string) (string, error) {
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
	log.Printf("[Outlook Graph] 等待验证码, 邮箱=%s, 起始时间=%s", acc.Email, after.Format(time.RFC3339))

	accessToken, err := RefreshOutlookGraphTokenWithProxy(acc, runtimeProxyURL)
	if err != nil {
		return "", fmt.Errorf("刷新 Outlook Graph Token 失败: %v", err)
	}

	codeRegex := regexp.MustCompile(`\b(\d{6})\b`)
	maxRetries := timeout / interval
	if maxRetries < 1 {
		maxRetries = 1
	}
	for attempt := 1; attempt <= maxRetries; attempt++ {
		code, err := findGraphOTP(accessToken, after, codeRegex, runtimeProxyURL)
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
		time.Sleep(time.Duration(interval) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}

func findGraphOTP(accessToken string, after time.Time, codeRegex *regexp.Regexp, proxyURL string) (string, error) {
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
	params.Set("$select", "subject,bodyPreview,body,receivedDateTime")
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
