package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	httputil "reg_go/internal/http"
	"reg_go/internal/proxy"
)

const (
	emailnatorPollInterval  = 3
	emailnatorCreateRetries = 20
	emailnatorUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
)

var (
	emailnatorBaseURL     = "https://www.emailnator.com"
	emailnatorHomeURL     = emailnatorBaseURL + "/"
	emailnatorGenerateURL = emailnatorBaseURL + "/generate-email"
	emailnatorMessagesURL = emailnatorBaseURL + "/message-list"

	emailnatorCodeRegex   = regexp.MustCompile(`(^|[^0-9])([0-9]{6})([^0-9]|$)`)
	emailnatorTagRegex    = regexp.MustCompile(`<[^>]+>`)
	emailnatorScriptRegex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	emailnatorStyleRegex  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	emailnatorBreakRegex  = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>`)
	emailnatorSpaceRegex  = regexp.MustCompile(`\s+`)
)

// EmailnatorService 提供 Emailnator Gmail 风格临时邮箱能力。
type EmailnatorService struct {
	client     tls_client.HttpClient
	proxyURL   string
	headers    map[string]string
	cookies    map[string]string
	address    string
	checkedIDs map[string]struct{}
}

// NewEmailnatorService 创建 Emailnator 临时邮箱服务。
func NewEmailnatorService(proxyURL string) *EmailnatorService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client, err := httputil.NewTLSClientWithTimeout(runtimeProxyURL, true, int(emailRequestTimeout/time.Second))
	if err != nil {
		log.Printf("[Emailnator] 邮箱代理初始化失败: %v", err)
		client, _ = httputil.NewTLSClientWithTimeout("", true, int(emailRequestTimeout/time.Second))
	}
	return &EmailnatorService{
		client:     client,
		proxyURL:   runtimeProxyURL,
		headers:    make(map[string]string),
		cookies:    make(map[string]string),
		checkedIDs: make(map[string]struct{}),
	}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *EmailnatorService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[Emailnator] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 创建 Emailnator Gmail 地址并返回详细错误。
func (s *EmailnatorService) CreateWithError() (string, error) {
	if err := s.ensureClientReady(); err != nil {
		return "", err
	}

	var lastText string
	for attempt := 1; attempt <= emailnatorCreateRetries; attempt++ {
		body, status, err := s.postJSON(emailnatorGenerateURL, map[string]interface{}{
			"email": []string{"plusGmail"},
		})
		lastText = string(body)
		if err != nil {
			log.Printf("[Emailnator] 生成邮箱失败 (%d/%d): %v", attempt, emailnatorCreateRetries, err)
			time.Sleep(time.Duration(emailnatorPollInterval) * time.Second)
			continue
		}
		if status >= 400 {
			log.Printf("[Emailnator] 生成邮箱 HTTP %d (%d/%d)", status, attempt, emailnatorCreateRetries)
			time.Sleep(time.Duration(emailnatorPollInterval) * time.Second)
			continue
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("[Emailnator] 解析生成响应失败 (%d/%d): %v", attempt, emailnatorCreateRetries, err)
			continue
		}
		for _, candidate := range emailnatorEmailCandidates(payload["email"]) {
			if strings.HasSuffix(strings.ToLower(candidate), "@gmail.com") {
				s.address = candidate
				log.Printf("[Emailnator] 邮箱生成成功: %s", candidate)
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("Emailnator 未返回 Gmail 地址: %s", shortEmailnatorBody(lastText, 200))
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *EmailnatorService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if s.address == "" {
		return "", fmt.Errorf("Emailnator 邮箱未创建")
	}
	if err := s.ensureClientReady(); err != nil {
		return "", err
	}
	if intervalSec <= 0 {
		intervalSec = emailnatorPollInterval
	}

	log.Printf("[Emailnator] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		body, status, err := s.postJSON(emailnatorMessagesURL, map[string]string{"email": s.address})
		if err != nil || status != 200 {
			if attempt%5 == 0 {
				log.Printf("[Emailnator] 获取邮件列表失败: status=%d err=%v", status, err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		if code := extractEmailnatorCodeFromText(string(body)); code != "" && emailnatorLooksLikeAWSVerification("", "", string(body)) {
			return code, nil
		}

		var payload interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("[Emailnator] 解析邮件列表失败: %v", err)
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		messages := normalizeEmailnatorMessages(payload)
		if len(messages) == 0 {
			if attempt%5 == 0 {
				log.Printf("[Emailnator] 暂无新邮件")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		for _, message := range messages {
			messageID := emailnatorMessageID(message)
			if messageID == "" {
				continue
			}
			if _, ok := s.checkedIDs[messageID]; ok {
				continue
			}
			s.checkedIDs[messageID] = struct{}{}

			subject := emailnatorString(message, "subject", "title")
			sender := emailnatorString(message, "from", "sender")
			detailText := s.messageDetailText(messageID)
			combined := html.UnescapeString(subject + "\n" + sender + "\n" + string(body) + "\n" + detailText)
			if !emailnatorLooksLikeAWSVerification(subject, sender, combined) {
				continue
			}
			if code := extractEmailnatorCodeFromText(combined); code != "" {
				log.Printf("[Emailnator] 成功提取验证码: %s", code)
				return code, nil
			}
		}

		time.Sleep(time.Duration(intervalSec) * time.Second)
	}

	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *EmailnatorService) GetAddress() string {
	return s.address
}

func (s *EmailnatorService) ensureClientReady() error {
	if len(s.headers) > 0 {
		return nil
	}
	body, status, headers, err := s.get(emailnatorHomeURL, map[string]string{
		"User-Agent": emailnatorUserAgent,
		"Accept":     "text/html,*/*",
	})
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Emailnator 首页 HTTP %d: %s", status, shortEmailnatorBody(string(body), 200))
	}
	httputil.SaveCookies(s.cookies, headers)

	xsrf := ""
	if raw := s.cookies["XSRF-TOKEN"]; raw != "" {
		xsrf = decodeEmailnatorXSRF(raw)
	}

	s.headers = map[string]string{
		"User-Agent":       emailnatorUserAgent,
		"Accept":           "application/json, text/plain, */*",
		"X-Requested-With": "XMLHttpRequest",
		"Content-Type":     "application/json;charset=UTF-8",
		"Origin":           emailnatorBaseURL,
		"Referer":          emailnatorHomeURL,
	}
	if xsrf != "" {
		s.headers["X-XSRF-TOKEN"] = xsrf
	}
	return nil
}

func (s *EmailnatorService) refreshXSRFHeader() {
	if s.headers == nil {
		return
	}
	if raw := s.cookies["XSRF-TOKEN"]; raw != "" {
		s.headers["X-XSRF-TOKEN"] = decodeEmailnatorXSRF(raw)
	}
}

func (s *EmailnatorService) messageDetailText(messageID string) string {
	payloads := []map[string]string{
		{"email": s.address, "messageID": messageID},
		{"email": s.address, "id": messageID},
	}
	for _, payload := range payloads {
		body, status, err := s.postJSON(emailnatorMessagesURL, payload)
		if err == nil && status == 200 {
			text := string(body)
			if strings.TrimSpace(text) != "" {
				return emailnatorHTMLToText(text)
			}
		}
	}
	return ""
}

func (s *EmailnatorService) get(rawURL string, headers map[string]string) ([]byte, int, map[string][]string, error) {
	req, err := fhttp.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	httputil.SetHeaders(req, headers)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, resp.Header, err
	}
	return body, resp.StatusCode, resp.Header, nil
}

func (s *EmailnatorService) postJSON(rawURL string, payload interface{}) ([]byte, int, error) {
	body, _ := json.Marshal(payload)
	req, err := fhttp.NewRequest("POST", rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	headers := make(map[string]string, len(s.headers)+1)
	for key, value := range s.headers {
		headers[key] = value
	}
	if cookieHeader := s.cookieHeader(); cookieHeader != "" {
		headers["Cookie"] = cookieHeader
	}
	httputil.SetHeaders(req, headers)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	httputil.SaveCookies(s.cookies, resp.Header)
	s.refreshXSRFHeader()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return respBody, resp.StatusCode, nil
}

func (s *EmailnatorService) cookieHeader() string {
	if len(s.cookies) == 0 {
		return ""
	}
	names := []string{"XSRF-TOKEN", "gmailnator_session"}
	parts := make([]string, 0, len(s.cookies))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if value := strings.TrimSpace(s.cookies[name]); value != "" {
			parts = append(parts, name+"="+value)
			seen[name] = true
		}
	}
	for name, value := range s.cookies {
		if seen[name] || strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, "; ")
}

func decodeEmailnatorXSRF(raw string) string {
	if decoded, err := url.QueryUnescape(raw); err == nil {
		return decoded
	}
	return raw
}

func emailnatorEmailCandidates(raw interface{}) []string {
	switch items := raw.(type) {
	case []interface{}:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return append([]string(nil), items...)
	case string:
		return []string{strings.TrimSpace(items)}
	default:
		return nil
	}
}

func normalizeEmailnatorMessages(payload interface{}) []map[string]interface{} {
	if items, ok := payload.([]interface{}); ok {
		return emailnatorMapItems(items)
	}

	obj, ok := payload.(map[string]interface{})
	if !ok {
		return nil
	}

	for _, key := range []string{"messageData", "data", "messages", "items"} {
		switch value := obj[key].(type) {
		case []interface{}:
			return emailnatorMapItems(value)
		case map[string]interface{}:
			for _, nestedKey := range []string{"messages", "data", "items"} {
				if nested, ok := value[nestedKey].([]interface{}); ok {
					return emailnatorMapItems(nested)
				}
			}
		}
	}
	return nil
}

func emailnatorMapItems(items []interface{}) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if msg, ok := item.(map[string]interface{}); ok {
			messages = append(messages, msg)
		}
	}
	return messages
}

func emailnatorMessageID(message map[string]interface{}) string {
	for _, key := range []string{"messageID", "id", "msgid", "uid"} {
		if value, ok := message[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func emailnatorString(detail map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		switch value := detail[key].(type) {
		case string:
			if value != "" {
				return value
			}
		case map[string]interface{}:
			for _, subKey := range []string{"address", "email", "name"} {
				if text, ok := value[subKey].(string); ok && text != "" {
					return text
				}
			}
		}
	}
	return ""
}

func emailnatorLooksLikeAWSVerification(subject, sender, content string) bool {
	lower := strings.ToLower(sender + "\n" + subject + "\n" + content)
	hints := []string{
		"signin.aws",
		"aws.amazon.com",
		"aws builder id",
		"verify your aws",
		"verification code",
		"验证码",
		"amazon q",
	}
	for _, hint := range hints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func extractEmailnatorCodeFromText(text string) string {
	plain := emailnatorHTMLToText(text)
	match := emailnatorCodeRegex.FindStringSubmatch(plain)
	if len(match) > 2 && match[2] != "000000" {
		return match[2]
	}
	return ""
}

func emailnatorHTMLToText(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}
	text := emailnatorStyleRegex.ReplaceAllString(rawHTML, "")
	text = emailnatorScriptRegex.ReplaceAllString(text, "")
	text = emailnatorBreakRegex.ReplaceAllString(text, "\n")
	text = emailnatorTagRegex.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return strings.TrimSpace(emailnatorSpaceRegex.ReplaceAllString(text, " "))
}

func shortEmailnatorBody(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit]
}
