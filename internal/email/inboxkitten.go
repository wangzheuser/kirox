package email

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	httputil "reg_go/internal/http"
	"reg_go/internal/proxy"
)

const inboxKittenUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"

var inboxKittenAPIBaseURL = "https://inboxkitten.com/api/v1/mail"

// InboxKittenService 提供 InboxKitten 零配置临时邮箱能力。
type InboxKittenService struct {
	client     tls_client.HttpClient
	address    string
	checkedIDs map[string]struct{}
}

// NewInboxKittenService 创建 InboxKitten 临时邮箱服务。
func NewInboxKittenService(proxyURL string) *InboxKittenService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client, err := httputil.NewTLSClientWithTimeout(runtimeProxyURL, true, int(emailRequestTimeout/time.Second))
	if err != nil {
		log.Printf("[InboxKitten] 邮箱代理初始化失败: %v", err)
		client, _ = httputil.NewTLSClientWithTimeout("", true, int(emailRequestTimeout/time.Second))
	}
	return &InboxKittenService{client: client, checkedIDs: make(map[string]struct{})}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *InboxKittenService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[InboxKitten] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 生成 InboxKitten 收件箱地址。
func (s *InboxKittenService) CreateWithError() (string, error) {
	local := randomInboxKittenLocalPart()
	if local == "" {
		return "", fmt.Errorf("生成 InboxKitten 邮箱名失败")
	}
	s.address = local + "@inboxkitten.com"
	log.Printf("[InboxKitten] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *InboxKittenService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("InboxKitten 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = mailGWPollInterval
	}

	log.Printf("[InboxKitten] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		emails, err := s.listEmails()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[InboxKitten] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if len(emails) == 0 {
			if attempt%5 == 0 {
				log.Printf("[InboxKitten] 暂无新邮件")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		for _, msg := range emails {
			messageID := inboxKittenMessageID(msg)
			if messageID == "" {
				messageID = fmt.Sprintf("%s|%s", inboxKittenHeader(msg, "from"), inboxKittenHeader(msg, "subject"))
			}
			if _, ok := s.checkedIDs[messageID]; ok {
				continue
			}
			s.checkedIDs[messageID] = struct{}{}

			sender := inboxKittenHeader(msg, "from")
			subject := inboxKittenHeader(msg, "subject")
			log.Printf("[InboxKitten] 发现邮件 - 发件人: %s, 主题: %s", sender, subject)
			if !mailGWLooksLikeAWSVerification(subject, sender, "") {
				continue
			}

			html, err := s.getMessageHTML(msg)
			if err != nil {
				log.Printf("[InboxKitten] 获取邮件正文失败: %v", err)
				continue
			}
			combined := strings.Join([]string{subject, sender, html, mailGWHTMLToText(html)}, "\n")
			if !mailGWLooksLikeAWSVerification(subject, sender, combined) {
				continue
			}
			if code := mailGWCodeFromText(combined); code != "" {
				log.Printf("[InboxKitten] 成功提取验证码: %s", code)
				return code, nil
			}
		}

		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *InboxKittenService) GetAddress() string { return s.address }

func (s *InboxKittenService) listEmails() ([]map[string]interface{}, error) {
	recipient := inboxKittenRecipient(s.address)
	rawURL := s.apiURL("/list") + "?recipient=" + url.QueryEscape(recipient)
	body, status, err := s.get(rawURL)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("获取 InboxKitten 邮件列表 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 InboxKitten 邮件列表失败: %w", err)
	}
	return normalizeMailGWMessages(payload), nil
}

func (s *InboxKittenService) getMessageHTML(message map[string]interface{}) (string, error) {
	region, key := inboxKittenStorage(message)
	if region == "" || key == "" {
		return "", fmt.Errorf("InboxKitten 邮件缺少 storage.region/key")
	}
	rawURL := s.apiURL("/getHtml") + "?region=" + url.QueryEscape(region) + "&key=" + url.QueryEscape(key)
	body, status, err := s.get(rawURL)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("获取 InboxKitten 邮件正文 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	return string(body), nil
}

func (s *InboxKittenService) apiURL(path string) string {
	base := strings.TrimRight(inboxKittenAPIBaseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (s *InboxKittenService) get(rawURL string) ([]byte, int, error) {
	req, err := fhttp.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	httputil.SetHeaders(req, inboxKittenHeaders())
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

func inboxKittenHeaders() map[string]string {
	return map[string]string{
		"Accept":          "application/json, text/plain, */*",
		"User-Agent":      inboxKittenUserAgent,
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
		"Referer":         "https://inboxkitten.com/",
	}
}

func inboxKittenRecipient(address string) string {
	address = strings.TrimSpace(strings.ToLower(address))
	if local, _, ok := strings.Cut(address, "@"); ok {
		return local
	}
	return address
}

func inboxKittenHeader(message map[string]interface{}, key string) string {
	msg, _ := message["message"].(map[string]interface{})
	headers, _ := msg["headers"].(map[string]interface{})
	if text, ok := headers[key].(string); ok {
		return text
	}
	return mailGWString(message, key)
}

func inboxKittenStorage(message map[string]interface{}) (string, string) {
	storage, _ := message["storage"].(map[string]interface{})
	region := strings.TrimSpace(fmt.Sprint(storage["region"]))
	key := strings.TrimSpace(fmt.Sprint(storage["key"]))
	if region == "<nil>" {
		region = ""
	}
	if key == "<nil>" {
		key = ""
	}
	return region, key
}

func inboxKittenMessageID(message map[string]interface{}) string {
	region, key := inboxKittenStorage(message)
	if region != "" || key != "" {
		return region + ":" + key
	}
	for _, field := range []string{"url", "id", "key"} {
		if text := strings.TrimSpace(fmt.Sprint(message[field])); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func randomInboxKittenLocalPart() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err == nil {
		return "kiro" + hex.EncodeToString(buf)
	}
	return fmt.Sprintf("kiro%d", time.Now().UnixNano())
}
