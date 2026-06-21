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

const freeCustomPollInterval = 3

var (
	freeCustomSiteBaseURL     = "https://www.freecustom.email"
	freeCustomDomainIx        int
	freeCustomFallbackDomains = []string{
		"ditapi.info",
		"ditcloud.info",
		"ditdrive.info",
		"ditgame.info",
		"ditlearn.info",
		"ditpay.info",
		"ditplay.info",
		"ditube.info",
		"ditmail.info",
		"fce.email",
	}
)

// FreeCustomService 提供 FreeCustom.Email 零配置临时邮箱能力。
type FreeCustomService struct {
	client     *http.Client
	baseURL    string
	address    string
	token      string
	checkedIDs map[string]struct{}
}

// NewFreeCustomService 创建 FreeCustom.Email 临时邮箱服务。
func NewFreeCustomService(proxyURL string) *FreeCustomService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	return &FreeCustomService{
		client:     httpClientWithProxy(runtimeProxyURL, emailRequestTimeout),
		baseURL:    freeCustomSiteBaseURL,
		checkedIDs: make(map[string]struct{}),
	}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *FreeCustomService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[FreeCustom] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 获取临时访问 token，并基于公开 free 域名生成邮箱地址。
func (s *FreeCustomService) CreateWithError() (string, error) {
	if err := s.ensureToken(); err != nil {
		return "", err
	}
	domains, err := s.getDomains()
	if err != nil {
		log.Printf("[FreeCustom] 获取域名池失败，使用内置域名: %v", err)
		domains = append([]string(nil), freeCustomFallbackDomains...)
	}
	if len(domains) == 0 {
		domains = append([]string(nil), freeCustomFallbackDomains...)
	}
	domain := nextFreeCustomDomain(domains)
	if domain == "" {
		return "", fmt.Errorf("FreeCustom 没有可用域名")
	}
	local := sanitizeFreeCustomLocal(GenerateEmailName(time.Now().Nanosecond()))
	if local == "" {
		return "", fmt.Errorf("FreeCustom 生成邮箱名前缀失败")
	}
	s.address = strings.ToLower(local + "@" + domain)
	if _, err := s.listMessages(); err != nil {
		return "", fmt.Errorf("初始化 FreeCustom 邮箱失败: %w", err)
	}
	log.Printf("[FreeCustom] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *FreeCustomService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("FreeCustom 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = freeCustomPollInterval
	}
	if err := s.ensureToken(); err != nil {
		return "", err
	}
	log.Printf("[FreeCustom] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.listMessages()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[FreeCustom] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if len(messages) == 0 {
			if attempt%5 == 0 {
				log.Printf("[FreeCustom] 暂无新邮件")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		for _, msg := range messages {
			id := freeCustomMessageID(msg)
			if id == "" {
				id = fmt.Sprintf("%s|%s|%s", mailGWString(msg, "from", "sender"), mailGWString(msg, "subject", "title"), mailGWString(msg, "date", "createdAt"))
			}
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			sender := mailGWString(msg, "from", "sender")
			subject := mailGWString(msg, "subject", "title")
			log.Printf("[FreeCustom] 发现邮件 - 发件人: %s, 主题: %s", sender, subject)
			combined := mailGWDetailText(msg)
			if id != "" {
				if detail, err := s.getMessageDetail(id); err == nil {
					combined += "\n" + mailGWDetailText(detail)
					if sender == "" {
						sender = mailGWString(detail, "from", "sender")
					}
					if subject == "" {
						subject = mailGWString(detail, "subject", "title")
					}
				} else {
					log.Printf("[FreeCustom] 获取邮件详情失败: %v", err)
				}
			}
			if !mailGWLooksLikeAWSVerification(subject, sender, combined) {
				continue
			}
			if code := mailGWCodeFromText(combined); code != "" {
				log.Printf("[FreeCustom] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *FreeCustomService) GetAddress() string { return s.address }

func (s *FreeCustomService) ensureToken() error {
	if strings.TrimSpace(s.token) != "" {
		return nil
	}
	body, status, err := s.request("POST", "/api/auth", nil, false)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("FreeCustom auth HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("解析 FreeCustom auth 失败: %w", err)
	}
	token := strings.TrimSpace(fmt.Sprint(data["token"]))
	if token == "" || token == "<nil>" {
		return fmt.Errorf("FreeCustom auth 响应缺少 token")
	}
	s.token = token
	return nil
}

func (s *FreeCustomService) getDomains() ([]string, error) {
	body, status, err := s.request("GET", "/api/domains", nil, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("FreeCustom domains HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 FreeCustom domains 失败: %w", err)
	}
	domains := make([]string, 0)
	items, _ := data["data"].([]interface{})
	for _, item := range items {
		obj, _ := item.(map[string]interface{})
		domain := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["domain"])))
		tier := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["tier"])))
		if domain == "" || domain == "<nil>" {
			continue
		}
		if tier != "" && tier != "<nil>" && tier != "free" {
			continue
		}
		domains = append(domains, domain)
	}
	return domains, nil
}

func (s *FreeCustomService) listMessages() ([]map[string]interface{}, error) {
	path := "/api/public-mailbox?fullMailboxId=" + url.QueryEscape(s.address)
	body, status, err := s.request("GET", path, nil, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("FreeCustom 邮件列表 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 FreeCustom 邮件列表失败: %w", err)
	}
	return normalizeMailGWMessages(data), nil
}

func (s *FreeCustomService) getMessageDetail(id string) (map[string]interface{}, error) {
	path := "/api/public-mailbox?fullMailboxId=" + url.QueryEscape(s.address) + "&messageId=" + url.QueryEscape(id)
	body, status, err := s.request("GET", path, nil, true)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("FreeCustom 邮件详情 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("解析 FreeCustom 邮件详情失败: %w", err)
	}
	if detail, ok := data["data"].(map[string]interface{}); ok {
		return detail, nil
	}
	return data, nil
}

func (s *FreeCustomService) request(method, path string, payload interface{}, auth bool) ([]byte, int, error) {
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
	req.Header.Set("Referer", "https://www.freecustom.email/en")
	req.Header.Set("x-fce-client", "web-client")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth && strings.TrimSpace(s.token) != "" {
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

func freeCustomMessageID(message map[string]interface{}) string {
	for _, key := range []string{"id", "_id", "uid", "message_id", "messageId"} {
		if v := strings.TrimSpace(fmt.Sprint(message[key])); v != "" && v != "<nil>" {
			return v
		}
	}
	return ""
}

func nextFreeCustomDomain(domains []string) string {
	if len(domains) == 0 {
		return ""
	}
	idx := freeCustomDomainIx % len(domains)
	freeCustomDomainIx++
	return strings.ToLower(strings.TrimSpace(domains[idx]))
}

func sanitizeFreeCustomLocal(local string) string {
	local = strings.ToLower(strings.TrimSpace(local))
	var b strings.Builder
	for _, r := range local {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "._-")
}
