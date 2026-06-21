package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"reg_go/internal/proxy"
)

const inboxesPollInterval = 3

var (
	inboxesAPIBaseURL = "https://inboxes.com/api/v2"
	inboxesDomainIx   int
)

// InboxesService 提供 Inboxes.com/Nada 零配置临时邮箱能力。
type InboxesService struct {
	client     *http.Client
	baseURL    string
	address    string
	token      string
	checkedIDs map[string]struct{}
}

// NewInboxesService 创建 Inboxes 临时邮箱服务。
func NewInboxesService(proxyURL string) *InboxesService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	return &InboxesService{
		client:     httpClientWithProxy(runtimeProxyURL, emailRequestTimeout),
		baseURL:    inboxesAPIBaseURL,
		checkedIDs: make(map[string]struct{}),
	}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *InboxesService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[Inboxes] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 注册匿名 Inboxes 会话并生成一个自定义收件箱地址。
func (s *InboxesService) CreateWithError() (string, error) {
	if err := s.signup(); err != nil {
		return "", err
	}
	domains, randomUser, err := s.getDomains()
	if err != nil {
		return "", err
	}
	if len(domains) == 0 {
		return "", fmt.Errorf("Inboxes 未返回可用域名")
	}
	local := strings.TrimSpace(randomUser)
	if local == "" {
		local = GenerateEmailName(time.Now().Nanosecond())
	}
	local = sanitizeInboxesLocal(local)
	if local == "" {
		local = GenerateEmailName(time.Now().Nanosecond())
	}
	domain := nextInboxesDomain(domains)
	s.address = strings.ToLower(local + "@" + domain)
	log.Printf("[Inboxes] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *InboxesService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" || strings.TrimSpace(s.token) == "" {
		return "", fmt.Errorf("Inboxes 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = inboxesPollInterval
	}
	log.Printf("[Inboxes] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.listMessages()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[Inboxes] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if len(messages) == 0 {
			if attempt%5 == 0 {
				log.Printf("[Inboxes] 暂无新邮件")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		for _, msg := range messages {
			id := inboxesMessageID(msg)
			if id == "" {
				id = fmt.Sprintf("%s|%s", mailGWString(msg, "f", "from", "sender"), mailGWString(msg, "s", "subject", "title"))
			}
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			sender := mailGWString(msg, "f", "from", "sender")
			subject := mailGWString(msg, "s", "subject", "title")
			preview := mailGWString(msg, "ph", "preview", "intro", "text")
			log.Printf("[Inboxes] 发现邮件 - 发件人: %s, 主题: %s", sender, subject)
			if !mailGWLooksLikeAWSVerification(subject, sender, preview) {
				continue
			}
			combined := strings.Join([]string{sender, subject, preview}, "\n")
			if id != "" {
				if detail, err := s.getMessageDetail(id); err == nil {
					combined += "\n" + mailGWDetailText(detail)
				} else {
					log.Printf("[Inboxes] 获取邮件详情失败: %v", err)
				}
			}
			if !mailGWLooksLikeAWSVerification(subject, sender, combined) {
				continue
			}
			if code := mailGWCodeFromText(combined); code != "" {
				log.Printf("[Inboxes] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *InboxesService) GetAddress() string { return s.address }

func (s *InboxesService) signup() error {
	payload := map[string]string{"nada_user": "kiro" + GenerateEmailName(time.Now().Nanosecond())}
	body, status, err := s.request("POST", "/signup", payload, false)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("Inboxes signup HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("解析 Inboxes signup 失败: %w", err)
	}
	token := strings.TrimSpace(fmt.Sprint(data["token"]))
	if token == "" || token == "<nil>" {
		return fmt.Errorf("Inboxes signup 响应缺少 token")
	}
	s.token = token
	return nil
}

func (s *InboxesService) getDomains() ([]string, string, error) {
	body, status, err := s.request("GET", "/domain", nil, true)
	if err != nil {
		return nil, "", err
	}
	if status != http.StatusOK {
		return nil, "", fmt.Errorf("Inboxes domain HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, "", fmt.Errorf("解析 Inboxes domain 失败: %w", err)
	}
	domains := make([]string, 0)
	if items, ok := data["domains"].([]interface{}); ok {
		for _, item := range items {
			if obj, ok := item.(map[string]interface{}); ok {
				if qdn := strings.TrimSpace(fmt.Sprint(obj["qdn"])); qdn != "" && qdn != "<nil>" {
					domains = append(domains, strings.ToLower(qdn))
				}
			}
		}
	}
	randomUser := strings.TrimSpace(fmt.Sprint(data["randomUser"]))
	if randomUser == "<nil>" {
		randomUser = ""
	}
	return domains, randomUser, nil
}

func (s *InboxesService) listMessages() ([]map[string]interface{}, error) {
	path := "/inbox/" + url.PathEscape(s.address)
	body, status, err := s.request("GET", path, nil, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("Inboxes 邮件列表 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 Inboxes 邮件列表失败: %w", err)
	}
	return normalizeMailGWMessages(data), nil
}

func (s *InboxesService) getMessageDetail(id string) (map[string]interface{}, error) {
	body, status, err := s.request("GET", "/message/"+url.PathEscape(id), nil, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("Inboxes 邮件详情 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 Inboxes 邮件详情失败: %w", err)
	}
	return data, nil
}

func (s *InboxesService) request(method, path string, payload interface{}, auth bool) ([]byte, int, error) {
	rawURL := strings.TrimRight(s.baseURL, "/") + path
	var body io.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Referer", "https://inboxes.com/")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth && s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return respBody, resp.StatusCode, nil
}

func inboxesMessageID(message map[string]interface{}) string {
	for _, key := range []string{"uid", "id", "messageID"} {
		if text := strings.TrimSpace(fmt.Sprint(message[key])); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func sanitizeInboxesLocal(local string) string {
	local = strings.ToLower(strings.TrimSpace(local))
	var b strings.Builder
	for _, r := range local {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ".-_")
}

func nextInboxesDomain(domains []string) string {
	idx := inboxesDomainIx % len(domains)
	inboxesDomainIx++
	return domains[idx]
}
