package email

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	mathrand "math/rand"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	httputil "reg_go/internal/http"
	"reg_go/internal/proxy"
)

const (
	mailporaryPageURL       = "https://mailporary.com/zh"
	mailporaryAPIBaseURL    = "https://web.mailporary.com/api/v1"
	mailporaryPollInterval  = 3
	mailporaryCreateRetries = 5
	mailporaryUserAgent     = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"
)

var (
	mailporaryFallbackDomains = []string{
		"oeralb.com",
		"sisood.com",
		"disefl.com",
		"suarj.com",
		"mfxis.com",
		"anogz.com",
	}
	mailporaryJWTRegex      = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
	mailporaryNuxtDataRegex = regexp.MustCompile(`(?is)<script[^>]+id=["']__NUXT_DATA__["'][^>]*>(.*?)</script>`)
	mailporaryDomainRegex   = regexp.MustCompile(`@([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)
	mailporaryCodeRegex     = regexp.MustCompile(`(^|[^0-9])([0-9]{6})([^0-9]|$)`)
	mailporaryTagRegex      = regexp.MustCompile(`<[^>]+>`)
	mailporaryStyleRegex    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	mailporaryScriptRegex   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	mailporaryBreakRegex    = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</div>`)
	mailporarySpaceRegex    = regexp.MustCompile(`\s+`)
	mailporaryUsedEmails    = map[string]struct{}{}
	mailporaryUsedMu        sync.Mutex
)

// MailporaryService 提供 Mailporary 零配置临时邮箱能力。
type MailporaryService struct {
	client     tls_client.HttpClient
	token      string
	address    string
	checkedIDs map[string]struct{}
}

// NewMailporaryService 创建 Mailporary 临时邮箱服务。
func NewMailporaryService(proxyURL string) *MailporaryService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client, err := httputil.NewTLSClientWithTimeout(runtimeProxyURL, true, int(emailRequestTimeout/time.Second))
	if err != nil {
		log.Printf("[Mailporary] 邮箱代理初始化失败: %v", err)
		client, _ = httputil.NewTLSClientWithTimeout("", true, int(emailRequestTimeout/time.Second))
	}
	return &MailporaryService{
		client:     client,
		checkedIDs: make(map[string]struct{}),
	}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *MailporaryService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[Mailporary] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 创建临时邮箱并返回详细错误。
func (s *MailporaryService) CreateWithError() (string, error) {
	log.Println("[Mailporary] 开始生成临时邮箱")

	token, err := s.fetchToken()
	if err != nil {
		return "", err
	}
	s.token = token

	domains := s.getAvailableDomains()
	if len(domains) == 0 {
		return "", fmt.Errorf("没有可用域名")
	}

	for attempt := 1; attempt <= mailporaryCreateRetries; attempt++ {
		emailAddress := fmt.Sprintf("%s@%s", s.generateLocalPart(), domains[mathrand.Intn(len(domains))])
		emailAddress = strings.ToLower(emailAddress)

		if s.isEmailUsed(emailAddress) {
			log.Printf("[Mailporary] 邮箱已使用，重试生成 (%d/%d): %s", attempt, mailporaryCreateRetries, emailAddress)
			continue
		}

		if err := s.createMailbox(emailAddress); err != nil {
			log.Printf("[Mailporary] 创建邮箱失败 (%d/%d): %v", attempt, mailporaryCreateRetries, err)
			continue
		}

		s.markEmailUsed(emailAddress)
		s.address = emailAddress
		log.Printf("[Mailporary] 邮箱生成成功: %s", emailAddress)
		return emailAddress, nil
	}

	return "", fmt.Errorf("邮箱生成失败，已重试 %d 次", mailporaryCreateRetries)
}

// WaitForCode 轮询等待 AWS 验证码。
func (s *MailporaryService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if s.address == "" || s.token == "" {
		return "", fmt.Errorf("Mailporary 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = mailporaryPollInterval
	}

	log.Printf("[Mailporary] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.listMessages()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[Mailporary] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		if len(messages) == 0 {
			if attempt%5 == 0 {
				log.Printf("[Mailporary] 暂无新邮件，继续等待")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		for _, msg := range messages {
			messageID := getMailporaryMessageID(msg)
			if messageID == "" {
				continue
			}
			if _, ok := s.checkedIDs[messageID]; ok {
				continue
			}
			s.checkedIDs[messageID] = struct{}{}

			detail, err := s.getMessageDetail(messageID)
			if err != nil {
				log.Printf("[Mailporary] 获取邮件详情失败 (%s): %v", messageID, err)
				continue
			}

			sender := mailporarySender(detail)
			subject := mailporaryString(detail, "subject", "title")
			log.Printf("[Mailporary] 发现邮件 - 发件人: %s, 主题: %s", sender, subject)

			if !isMailporaryAWSVerificationEmail(detail) {
				continue
			}

			code := extractMailporaryCode(detail)
			if code == "" || code == "000000" {
				continue
			}

			log.Printf("[Mailporary] 成功提取验证码: %s", code)
			return code, nil
		}

		time.Sleep(time.Duration(intervalSec) * time.Second)
	}

	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *MailporaryService) GetAddress() string {
	return s.address
}

// fetchToken 从 Mailporary 页面提取接口 token。
func (s *MailporaryService) fetchToken() (string, error) {
	body, status, err := s.get(mailporaryPageURL, map[string]string{
		"User-Agent":      mailporaryUserAgent,
		"Accept-Language": "zh-CN,zh;q=0.9",
	})
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("获取页面失败，状态码: %d", status)
	}

	matches := mailporaryNuxtDataRegex.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("未找到 __NUXT_DATA__，页面结构可能已变更")
	}

	return parseMailporaryToken(matches[1])
}

// getAvailableDomains 获取可用域名，失败时回退内置域名。
func (s *MailporaryService) getAvailableDomains() []string {
	probeEmail := fmt.Sprintf("probe%s@%s", randomHex(4), mailporaryFallbackDomains[0])
	body, status, err := s.get(s.mailboxURL(probeEmail), s.apiHeaders())
	if err != nil || status != 200 {
		log.Printf("[Mailporary] 动态获取域名失败，使用内置域名: status=%d err=%v", status, err)
		return append([]string(nil), mailporaryFallbackDomains...)
	}

	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[Mailporary] 解析域名响应失败，使用内置域名: %v", err)
		return append([]string(nil), mailporaryFallbackDomains...)
	}

	domains := extractMailporaryDomains(payload)
	if len(domains) == 0 {
		return append([]string(nil), mailporaryFallbackDomains...)
	}

	log.Printf("[Mailporary] 动态获取到 %d 个域名", len(domains))
	return domains
}

// createMailbox 创建或激活邮箱。
func (s *MailporaryService) createMailbox(emailAddress string) error {
	body, status, err := s.get(s.mailboxURL(emailAddress), s.apiHeaders())
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("创建/访问邮箱失败，状态码: %d, 响应: %s", status, string(body))
	}
	return nil
}

// listMessages 获取邮箱邮件列表。
func (s *MailporaryService) listMessages() ([]map[string]interface{}, error) {
	body, status, err := s.get(s.mailboxURL(s.address), s.apiHeaders())
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("获取邮件列表失败，状态码: %d", status)
	}

	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析邮件列表失败: %w", err)
	}

	return normalizeMailporaryMessages(payload), nil
}

// getMessageDetail 获取单封邮件详情。
func (s *MailporaryService) getMessageDetail(messageID string) (map[string]interface{}, error) {
	detailURL := fmt.Sprintf("%s/%s", s.mailboxURL(s.address), url.PathEscape(messageID))
	body, status, err := s.get(detailURL, s.apiHeaders())
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("获取邮件详情失败，状态码: %d", status)
	}

	var detail map[string]interface{}
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("解析邮件详情失败: %w", err)
	}
	return detail, nil
}

// get 发送 GET 请求。
func (s *MailporaryService) get(rawURL string, headers map[string]string) ([]byte, int, error) {
	req, err := fhttp.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, err
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

// apiHeaders 构造 Mailporary API 请求头。
func (s *MailporaryService) apiHeaders() map[string]string {
	return map[string]string{
		"Authorization":   "Bearer " + s.token,
		"Accept":          "*/*",
		"Content-Type":    "application/json",
		"Origin":          "https://mailporary.com",
		"Referer":         "https://mailporary.com/",
		"User-Agent":      mailporaryUserAgent,
		"Accept-Language": "zh-CN,zh;q=0.9",
		"X-Request-ID":    randomHex(16),
		"X-Timestamp":     fmt.Sprintf("%d", time.Now().Unix()),
	}
}

// mailboxURL 构造邮箱 API 地址。
func (s *MailporaryService) mailboxURL(emailAddress string) string {
	return fmt.Sprintf("%s/mailbox/%s", mailporaryAPIBaseURL, url.PathEscape(emailAddress))
}

// generateLocalPart 生成 3 到 8 位随机字母邮箱名前缀。
func (s *MailporaryService) generateLocalPart() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz"
	length := 3 + mathrand.Intn(6)
	var builder strings.Builder
	for i := 0; i < length; i++ {
		builder.WriteByte(alphabet[mathrand.Intn(len(alphabet))])
	}
	return builder.String()
}

// isEmailUsed 判断邮箱是否已在当前进程中使用过。
func (s *MailporaryService) isEmailUsed(emailAddress string) bool {
	mailporaryUsedMu.Lock()
	defer mailporaryUsedMu.Unlock()
	_, ok := mailporaryUsedEmails[emailAddress]
	return ok
}

// markEmailUsed 标记邮箱已使用。
func (s *MailporaryService) markEmailUsed(emailAddress string) {
	mailporaryUsedMu.Lock()
	defer mailporaryUsedMu.Unlock()
	mailporaryUsedEmails[emailAddress] = struct{}{}
}

// parseMailporaryToken 从 __NUXT_DATA__ 中解析 Mailporary token。
func parseMailporaryToken(nuxtData []byte) (string, error) {
	var parsed []interface{}
	if err := json.Unmarshal(nuxtData, &parsed); err != nil {
		return "", fmt.Errorf("解析 __NUXT_DATA__ 失败: %w", err)
	}
	if len(parsed) < 2 {
		return "", fmt.Errorf("__NUXT_DATA__ 结构异常")
	}

	if root, ok := parsed[1].(map[string]interface{}); ok {
		if rawIndex, ok := root["mailServiceToken"].(float64); ok {
			index := int(rawIndex)
			if index >= 0 && index < len(parsed) {
				if tokenText, ok := parsed[index].(string); ok {
					if token := mailporaryJWTRegex.FindString(tokenText); token != "" {
						return token, nil
					}
				}
			}
		}
	}

	for _, item := range parsed {
		if tokenText, ok := item.(string); ok {
			if token := mailporaryJWTRegex.FindString(tokenText); token != "" {
				return token, nil
			}
		}
	}

	return "", fmt.Errorf("__NUXT_DATA__ 中未找到可用 token")
}

// extractMailporaryDomains 从任意响应结构中提取邮箱域名。
func extractMailporaryDomains(payload interface{}) []string {
	domainSet := map[string]struct{}{}
	var scan func(interface{})

	scan = func(value interface{}) {
		switch v := value.(type) {
		case string:
			for _, match := range mailporaryDomainRegex.FindAllStringSubmatch(v, -1) {
				if len(match) > 1 {
					domainSet[strings.ToLower(match[1])] = struct{}{}
				}
			}
		case []interface{}:
			for _, item := range v {
				scan(item)
			}
		case map[string]interface{}:
			for _, item := range v {
				scan(item)
			}
		}
	}

	scan(payload)
	domains := make([]string, 0, len(domainSet))
	for domain := range domainSet {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	return domains
}

// normalizeMailporaryMessages 将不同列表响应结构归一化为邮件数组。
func normalizeMailporaryMessages(payload interface{}) []map[string]interface{} {
	if items, ok := payload.([]interface{}); ok {
		return mailporaryMapItems(items)
	}

	obj, ok := payload.(map[string]interface{})
	if !ok {
		return nil
	}

	for _, key := range []string{"messages", "items", "data", "mailbox", "list"} {
		if items, ok := obj[key].([]interface{}); ok {
			return mailporaryMapItems(items)
		}
	}

	return nil
}

// mailporaryMapItems 过滤出对象类型的邮件项。
func mailporaryMapItems(items []interface{}) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if msg, ok := item.(map[string]interface{}); ok {
			messages = append(messages, msg)
		}
	}
	return messages
}

// getMailporaryMessageID 提取邮件 ID。
func getMailporaryMessageID(message map[string]interface{}) string {
	if id, ok := message["id"].(string); ok {
		return id
	}
	if id, ok := message["uid"].(string); ok {
		return id
	}
	return ""
}

// isMailporaryAWSVerificationEmail 判断邮件是否为 AWS 验证邮件。
func isMailporaryAWSVerificationEmail(detail map[string]interface{}) bool {
	sender := mailporarySender(detail)
	subject := strings.ToLower(mailporaryString(detail, "subject", "title"))
	bodyText := mailporaryBodyString(detail, "text")
	bodyHTML := mailporaryBodyString(detail, "html")
	intro := mailporaryString(detail, "intro")
	content := strings.ToLower(subject + "\n" + intro + "\n" + bodyText + "\n" + mailporaryHTMLToText(bodyHTML))

	return strings.Contains(sender, "signin.aws") ||
		strings.Contains(sender, "aws.amazon.com") ||
		strings.Contains(subject, "aws builder id") ||
		strings.Contains(content, "verify your aws") ||
		strings.Contains(content, "verification code") ||
		strings.Contains(content, "验证码")
}

// extractMailporaryCode 从邮件详情中提取验证码。
func extractMailporaryCode(detail map[string]interface{}) string {
	candidates := []string{
		mailporaryString(detail, "subject", "title"),
		mailporaryString(detail, "intro"),
		mailporaryBodyString(detail, "text"),
		mailporaryHTMLToText(mailporaryBodyString(detail, "html")),
	}

	for _, text := range candidates {
		if code := mailporaryCodeFromText(text); code != "" {
			return code
		}
	}
	return ""
}

// mailporaryCodeFromText 从文本中提取独立 6 位验证码。
func mailporaryCodeFromText(text string) string {
	match := mailporaryCodeRegex.FindStringSubmatch(text)
	if len(match) > 2 {
		return match[2]
	}
	return ""
}

// mailporarySender 提取发件人地址。
func mailporarySender(detail map[string]interface{}) string {
	for _, key := range []string{"from", "sender"} {
		switch value := detail[key].(type) {
		case string:
			return strings.ToLower(value)
		case map[string]interface{}:
			for _, subKey := range []string{"address", "email", "name"} {
				if text, ok := value[subKey].(string); ok && text != "" {
					return strings.ToLower(text)
				}
			}
		}
	}
	return ""
}

// mailporaryString 按顺序读取第一个非空字符串字段。
func mailporaryString(detail map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if text, ok := detail[key].(string); ok && text != "" {
			return text
		}
	}
	return ""
}

// mailporaryBodyString 读取 body 下的文本字段，兼容顶层同名字段。
func mailporaryBodyString(detail map[string]interface{}, key string) string {
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

// mailporaryHTMLToText 将 HTML 内容转换为纯文本。
func mailporaryHTMLToText(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}
	text := mailporaryStyleRegex.ReplaceAllString(rawHTML, "")
	text = mailporaryScriptRegex.ReplaceAllString(text, "")
	text = mailporaryBreakRegex.ReplaceAllString(text, "\n")
	text = mailporaryTagRegex.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	return strings.TrimSpace(mailporarySpaceRegex.ReplaceAllString(text, " "))
}

// randomHex 生成指定字节数的十六进制字符串。
func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
