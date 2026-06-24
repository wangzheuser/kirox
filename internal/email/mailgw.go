package email

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	mathrand "math/rand"
	"net/http"
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
	mailGWPollInterval  = 3
	mailGWCreateRetries = 5
	mailGWUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
)

var (
	mailGWAPIBaseURL = "https://api.mail.gw"

	mailGWCodeRegex   = regexp.MustCompile(`(^|[^0-9])([0-9]{6})([^0-9]|$)`)
	mailGWTagRegex    = regexp.MustCompile(`<[^>]+>`)
	mailGWScriptRegex = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	mailGWStyleRegex  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	mailGWBreakRegex  = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>`)
	mailGWSpaceRegex  = regexp.MustCompile(`\s+`)
)

// MailGWService 提供 mail.gw 零配置临时邮箱能力。
type MailGWService struct {
	client      tls_client.HttpClient
	apiBaseURL  string
	displayName string
	address     string
	password    string
	token       string
	checkedIDs  map[string]struct{}
}

// NewMailGWService 创建 mail.gw 临时邮箱服务。
func NewMailGWService(proxyURL string) *MailGWService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client, err := httputil.NewTLSClientWithTimeout(runtimeProxyURL, true, int(emailRequestTimeout/time.Second))
	if err != nil {
		log.Printf("[mail.gw] 邮箱代理初始化失败: %s", proxy.SanitizeError(err, runtimeProxyURL))
		client, _ = httputil.NewTLSClientWithTimeout("", true, int(emailRequestTimeout/time.Second))
	}
	return &MailGWService{client: client, apiBaseURL: mailGWAPIBaseURL, displayName: "mail.gw", checkedIDs: make(map[string]struct{})}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *MailGWService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[%s] 创建邮箱失败: %v", s.displayLabel(), err)
		return ""
	}
	return address
}

// CreateWithError 创建 mail.gw 账号并获取访问 token。
func (s *MailGWService) CreateWithError() (string, error) {
	domains, err := s.getDomains()
	if err != nil {
		return "", err
	}
	if len(domains) == 0 {
		return "", fmt.Errorf("%s 没有可用域名", s.displayLabel())
	}

	var lastErr error
	for attempt := 1; attempt <= mailGWCreateRetries; attempt++ {
		domain := domains[mathrand.Intn(len(domains))]
		address := strings.ToLower(fmt.Sprintf("%s@%s", GenerateEmailName(attempt), domain))
		password := randomMailGWPassword()

		if err := s.createAccount(address, password); err != nil {
			lastErr = err
			log.Printf("[%s] 创建账号失败 (%d/%d): %v", s.displayLabel(), attempt, mailGWCreateRetries, err)
			continue
		}
		token, err := s.createToken(address, password)
		if err != nil {
			lastErr = err
			log.Printf("[%s] 获取 token 失败 (%d/%d): %v", s.displayLabel(), attempt, mailGWCreateRetries, err)
			continue
		}

		s.address = address
		s.password = password
		s.token = token
		log.Printf("[%s] 邮箱生成成功: %s", s.displayLabel(), address)
		return address, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("%s 邮箱生成失败，已重试 %d 次: %w", s.displayLabel(), mailGWCreateRetries, lastErr)
	}
	return "", fmt.Errorf("%s 邮箱生成失败，已重试 %d 次", s.displayLabel(), mailGWCreateRetries)
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *MailGWService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" || strings.TrimSpace(s.token) == "" {
		return "", fmt.Errorf("%s 邮箱未创建", s.displayLabel())
	}
	if intervalSec <= 0 {
		intervalSec = mailGWPollInterval
	}

	log.Printf("[%s] 开始等待验证码: %s", s.displayLabel(), s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.listMessages()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[%s] 获取邮件列表失败: %v", s.displayLabel(), err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if len(messages) == 0 {
			if attempt%5 == 0 {
				log.Printf("[%s] 暂无新邮件", s.displayLabel())
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		for _, msg := range messages {
			messageID := mailGWMessageID(msg)
			if messageID == "" {
				continue
			}
			if _, ok := s.checkedIDs[messageID]; ok {
				continue
			}
			s.checkedIDs[messageID] = struct{}{}

			detail, err := s.getMessageDetail(messageID)
			if err != nil {
				log.Printf("[%s] 获取邮件详情失败 (%s): %v", s.displayLabel(), messageID, err)
				continue
			}
			sender := mailGWString(detail, "from", "sender")
			subject := mailGWString(detail, "subject", "title")
			log.Printf("[%s] 发现邮件 - 发件人: %s, 主题: %s", s.displayLabel(), sender, subject)
			if !mailGWLooksLikeAWSVerification(subject, sender, mailGWDetailText(detail)) {
				continue
			}
			if code := extractMailGWCode(detail); code != "" {
				log.Printf("[%s] 成功提取验证码: %s", s.displayLabel(), code)
				return code, nil
			}
		}

		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *MailGWService) GetAddress() string {
	return s.address
}

func (s *MailGWService) apiURL(path string) string {
	base := strings.TrimRight(s.apiBaseURL, "/")
	if base == "" {
		base = mailGWAPIBaseURL
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (s *MailGWService) displayLabel() string {
	if strings.TrimSpace(s.displayName) != "" {
		return s.displayName
	}
	return "mail.gw"
}

func (s *MailGWService) getDomains() ([]string, error) {
	body, status, err := s.get(s.apiURL("/domains?page=1"), nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("获取 %s 域名失败 HTTP %d: %s", s.displayLabel(), status, shortMailGWBody(string(body), 200))
	}
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 %s 域名失败: %w", s.displayLabel(), err)
	}
	return normalizeMailGWDomains(payload), nil
}

func (s *MailGWService) createAccount(address, password string) error {
	body, status, err := s.postJSON(s.apiURL("/accounts"), map[string]string{"address": address, "password": password}, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return fmt.Errorf("创建账号 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	return nil
}

func (s *MailGWService) createToken(address, password string) (string, error) {
	body, status, err := s.postJSON(s.apiURL("/token"), map[string]string{"address": address, "password": password}, "")
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("获取 token HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析 token 响应失败: %w", err)
	}
	token := strings.TrimSpace(fmt.Sprint(payload["token"]))
	if token == "" || token == "<nil>" {
		return "", fmt.Errorf("token 响应缺少 token")
	}
	return token, nil
}

func (s *MailGWService) listMessages() ([]map[string]interface{}, error) {
	body, status, err := s.get(s.apiURL("/messages"), map[string]string{"Authorization": "Bearer " + s.token})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("获取邮件列表 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析邮件列表失败: %w", err)
	}
	return normalizeMailGWMessages(payload), nil
}

func (s *MailGWService) getMessageDetail(messageID string) (map[string]interface{}, error) {
	body, status, err := s.get(s.apiURL("/messages/"+url.PathEscape(messageID)), map[string]string{"Authorization": "Bearer " + s.token})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("获取邮件详情 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析邮件详情失败: %w", err)
	}
	return payload, nil
}

func (s *MailGWService) get(rawURL string, extraHeaders map[string]string) ([]byte, int, error) {
	req, err := fhttp.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	headers := mailGWHeaders()
	for key, value := range extraHeaders {
		headers[key] = value
	}
	httputil.SetHeaders(req, headers)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (s *MailGWService) postJSON(rawURL string, payload interface{}, bearerToken string) ([]byte, int, error) {
	reqBody, _ := json.Marshal(payload)
	req, err := fhttp.NewRequest("POST", rawURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, 0, err
	}
	headers := mailGWHeaders()
	headers["Content-Type"] = "application/json"
	if bearerToken != "" {
		headers["Authorization"] = "Bearer " + bearerToken
	}
	httputil.SetHeaders(req, headers)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func mailGWHeaders() map[string]string {
	return map[string]string{
		"Accept":          "application/ld+json, application/json, text/plain, */*",
		"User-Agent":      mailGWUserAgent,
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	}
}

func normalizeMailGWDomains(payload interface{}) []string {
	var items []interface{}
	switch v := payload.(type) {
	case []interface{}:
		items = v
	case map[string]interface{}:
		for _, key := range []string{"hydra:member", "data", "domains", "items"} {
			if arr, ok := v[key].([]interface{}); ok {
				items = arr
				break
			}
		}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		active, hasActive := obj["isActive"].(bool)
		if hasActive && !active {
			continue
		}
		domain := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["domain"])))
		if domain != "" && domain != "<nil>" {
			out = append(out, domain)
		}
	}
	return out
}

func normalizeMailGWMessages(payload interface{}) []map[string]interface{} {
	if items, ok := payload.([]interface{}); ok {
		return mailGWMapItems(items)
	}
	obj, ok := payload.(map[string]interface{})
	if !ok {
		return nil
	}
	for _, key := range []string{"hydra:member", "messages", "emails", "items", "data", "msgs"} {
		switch value := obj[key].(type) {
		case []interface{}:
			return mailGWMapItems(value)
		case map[string]interface{}:
			for _, nestedKey := range []string{"messages", "emails", "hydra:member", "items", "data", "msgs"} {
				if nested, ok := value[nestedKey].([]interface{}); ok {
					return mailGWMapItems(nested)
				}
			}
		}
	}
	return nil
}

func mailGWMapItems(items []interface{}) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if msg, ok := item.(map[string]interface{}); ok {
			messages = append(messages, msg)
		}
	}
	return messages
}

func mailGWMessageID(message map[string]interface{}) string {
	for _, key := range []string{"id", "@id", "messageID", "msgid", "uid"} {
		if text := strings.TrimSpace(fmt.Sprint(message[key])); text != "" && text != "<nil>" {
			return strings.Trim(strings.TrimPrefix(text, "/messages/"), "/")
		}
	}
	return ""
}

func mailGWLooksLikeAWSVerification(subject, sender, content string) bool {
	lower := strings.ToLower(sender + "\n" + subject + "\n" + content)
	for _, hint := range []string{"signin.aws", "aws.amazon.com", "aws builder id", "verify your aws", "verification code", "验证码", "amazon q"} {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func extractMailGWCode(detail map[string]interface{}) string {
	for _, text := range []string{
		mailGWString(detail, "subject", "title"),
		mailGWString(detail, "intro"),
		mailGWString(detail, "body", "content"),
		mailGWHTMLToText(mailGWString(detail, "body", "content")),
		mailGWBodyString(detail, "text"),
		mailGWHTMLToText(mailGWBodyString(detail, "html")),
		mailGWHTMLToText(mailGWBodyArrayString(detail, "html")),
	} {
		if code := mailGWCodeFromText(text); code != "" {
			return code
		}
	}
	return ""
}

func mailGWDetailText(detail map[string]interface{}) string {
	return strings.Join([]string{
		mailGWString(detail, "subject", "title"),
		mailGWString(detail, "intro"),
		mailGWString(detail, "body", "content"),
		mailGWHTMLToText(mailGWString(detail, "body", "content")),
		mailGWBodyString(detail, "text"),
		mailGWHTMLToText(mailGWBodyString(detail, "html")),
		mailGWHTMLToText(mailGWBodyArrayString(detail, "html")),
	}, "\n")
}

func mailGWCodeFromText(text string) string {
	match := mailGWCodeRegex.FindStringSubmatch(mailGWHTMLToText(text))
	if len(match) > 2 && match[2] != "000000" {
		return match[2]
	}
	return ""
}

func mailGWString(detail map[string]interface{}, keys ...string) string {
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

func mailGWBodyString(detail map[string]interface{}, key string) string {
	if body, ok := detail["body"].(map[string]interface{}); ok {
		if text, ok := body[key].(string); ok {
			return text
		}
	}
	if text, ok := detail[key].(string); ok {
		return text
	}
	return ""
}

func mailGWBodyArrayString(detail map[string]interface{}, key string) string {
	if items, ok := detail[key].([]interface{}); ok {
		parts := make([]string, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func mailGWHTMLToText(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}
	text := mailGWStyleRegex.ReplaceAllString(rawHTML, "")
	text = mailGWScriptRegex.ReplaceAllString(text, "")
	text = mailGWBreakRegex.ReplaceAllString(text, "\n")
	text = mailGWTagRegex.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return strings.TrimSpace(mailGWSpaceRegex.ReplaceAllString(text, " "))
}

func randomMailGWPassword() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return "Kx!" + hex.EncodeToString(buf)
	}
	return fmt.Sprintf("Kx!%d", time.Now().UnixNano())
}

func shortMailGWBody(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit]
}
