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

const tempMailPlusPollInterval = 3

var (
	tempMailPlusBaseURL     = "https://tempmail.plus"
	tempMailPlusDomains     = []string{"fextemp.com", "fexbox.org", "merepost.com", "rover.info", "fexpost.com", "mailto.plus"}
	tempMailPlusDomainIx    int
	tempMailPlusDomainBtnRe = regexp.MustCompile(`(?is)<button[^>]*class=["'][^"']*dropdown-item[^"']*["'][^>]*>\s*([^<\s]+)\s*</button>`)
)

// TempMailPlusService 提供 TempMail.plus 零配置临时邮箱能力。
type TempMailPlusService struct {
	client     *http.Client
	baseURL    string
	address    string
	checkedIDs map[string]struct{}
}

// NewTempMailPlusService 创建 TempMail.plus 临时邮箱服务。
func NewTempMailPlusService(proxyURL string) *TempMailPlusService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	return &TempMailPlusService{
		client:     httpClientWithProxy(runtimeProxyURL, emailRequestTimeout),
		baseURL:    tempMailPlusBaseURL,
		checkedIDs: make(map[string]struct{}),
	}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *TempMailPlusService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[TempMail.plus] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 生成一个 TempMail.plus 地址并探测邮箱 API。
func (s *TempMailPlusService) CreateWithError() (string, error) {
	domains := appendUniqueDomains(nil, s.discoverDomains()...)
	domains = appendUniqueDomains(domains, nextTempMailPlusDomains()...)
	var lastErr error
	for _, domain := range domains {
		address := strings.ToLower(fmt.Sprintf("%s@%s", GenerateEmailName(time.Now().Nanosecond()), domain))
		if _, err := s.listMessagesFor(address); err != nil {
			lastErr = err
			log.Printf("[TempMail.plus] 域名探测失败: %s (%v)", domain, err)
			continue
		}
		s.address = address
		log.Printf("[TempMail.plus] 邮箱生成成功: %s", address)
		return address, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("TempMail.plus 未配置可用域名")
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *TempMailPlusService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("TempMail.plus 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = tempMailPlusPollInterval
	}
	log.Printf("[TempMail.plus] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.listMessagesFor(s.address)
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[TempMail.plus] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		for _, msg := range messages {
			id := tempMailPlusMessageID(msg)
			if id == "" {
				continue
			}
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			combined := tempMailPlusDetailText(msg)
			if detail, err := s.getMessageDetail(id); err == nil {
				combined += "\n" + tempMailPlusDetailText(detail)
			}
			if !mailGWLooksLikeAWSVerification("", "", combined) {
				continue
			}
			if code := mailGWCodeFromText(combined); code != "" {
				log.Printf("[TempMail.plus] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *TempMailPlusService) GetAddress() string {
	return s.address
}

func (s *TempMailPlusService) discoverDomains() []string {
	body, status, err := s.get(strings.TrimRight(s.baseURL, "/") + "/")
	if err != nil || status != http.StatusOK {
		if err != nil {
			log.Printf("[TempMail.plus] 动态获取域名失败，使用内置域名: %v", err)
		} else {
			log.Printf("[TempMail.plus] 动态获取域名 HTTP %d，使用内置域名", status)
		}
		return nil
	}
	domains := extractTempMailPlusDomains(string(body))
	if len(domains) > 0 {
		log.Printf("[TempMail.plus] 动态获取到 %d 个域名", len(domains))
	}
	return domains
}

func (s *TempMailPlusService) listMessagesFor(address string) ([]map[string]interface{}, error) {
	rawURL, err := s.apiURL("/api/mails", map[string]string{"email": address, "limit": "20"})
	if err != nil {
		return nil, err
	}
	body, status, err := s.get(rawURL)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("TempMail.plus 邮件列表 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 TempMail.plus 邮件列表失败: %w", err)
	}
	return normalizeTempMailPlusMessages(payload), nil
}

func (s *TempMailPlusService) getMessageDetail(id string) (map[string]interface{}, error) {
	rawURL, err := s.apiURL("/api/mails/"+url.PathEscape(id), map[string]string{"email": s.address})
	if err != nil {
		return nil, err
	}
	body, status, err := s.get(rawURL)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("TempMail.plus 邮件详情 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 TempMail.plus 邮件详情失败: %w", err)
	}
	return payload, nil
}

func (s *TempMailPlusService) apiURL(path string, params map[string]string) (string, error) {
	u, err := url.Parse(strings.TrimRight(s.baseURL, "/") + path)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (s *TempMailPlusService) get(rawURL string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Referer", strings.TrimRight(s.baseURL, "/")+"/")
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

func normalizeTempMailPlusMessages(payload interface{}) []map[string]interface{} {
	switch data := payload.(type) {
	case []interface{}:
		return tempMailPlusMapItems(data)
	case map[string]interface{}:
		for _, key := range []string{"mail_list", "mails", "messages", "data"} {
			if items, ok := data[key].([]interface{}); ok {
				return tempMailPlusMapItems(items)
			}
		}
	}
	return nil
}

func extractTempMailPlusDomains(html string) []string {
	domains := []string{}
	for _, match := range tempMailPlusDomainBtnRe.FindAllStringSubmatch(html, -1) {
		if len(match) > 1 {
			domains = appendUniqueDomains(domains, match[1])
		}
	}
	return domains
}

func tempMailPlusMapItems(items []interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func tempMailPlusMessageID(message map[string]interface{}) string {
	for _, key := range []string{"mail_id", "id", "_id"} {
		if text := strings.TrimSpace(fmt.Sprint(message[key])); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func tempMailPlusDetailText(detail map[string]interface{}) string {
	return strings.Join([]string{
		mailGWString(detail, "from", "sender"),
		mailGWString(detail, "subject", "title"),
		mailGWString(detail, "text", "body", "html", "content"),
		mailGWHTMLToText(mailGWString(detail, "html", "body", "content")),
	}, "\n")
}

func nextTempMailPlusDomains() []string {
	if len(tempMailPlusDomains) == 0 {
		return nil
	}
	start := tempMailPlusDomainIx % len(tempMailPlusDomains)
	tempMailPlusDomainIx++
	out := make([]string, 0, len(tempMailPlusDomains))
	for i := 0; i < len(tempMailPlusDomains); i++ {
		out = append(out, tempMailPlusDomains[(start+i)%len(tempMailPlusDomains)])
	}
	return out
}
