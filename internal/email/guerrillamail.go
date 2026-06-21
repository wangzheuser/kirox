package email

import (
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

const (
	guerrillaMailPollInterval = 3
	guerrillaMailUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
)

var guerrillaMailAPIBaseURL = "https://api.guerrillamail.com/ajax.php"

// GuerrillaMailService 提供 GuerrillaMail 零配置临时邮箱能力。
type GuerrillaMailService struct {
	client     tls_client.HttpClient
	apiBaseURL string
	address    string
	sidToken   string
	checkedIDs map[string]struct{}
}

// NewGuerrillaMailService 创建 GuerrillaMail 临时邮箱服务。
func NewGuerrillaMailService(proxyURL string) *GuerrillaMailService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client, err := httputil.NewTLSClientWithTimeout(runtimeProxyURL, true, int(emailRequestTimeout/time.Second))
	if err != nil {
		log.Printf("[GuerrillaMail] 邮箱代理初始化失败: %v", err)
		client, _ = httputil.NewTLSClientWithTimeout("", true, int(emailRequestTimeout/time.Second))
	}
	return &GuerrillaMailService{client: client, apiBaseURL: guerrillaMailAPIBaseURL, checkedIDs: make(map[string]struct{})}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *GuerrillaMailService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[GuerrillaMail] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 生成 GuerrillaMail 邮箱并保存会话 token。
func (s *GuerrillaMailService) CreateWithError() (string, error) {
	body, status, err := s.get(map[string]string{"f": "get_email_address", "lang": "en"})
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("创建 GuerrillaMail 邮箱 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析 GuerrillaMail 创建响应失败: %w", err)
	}
	address := strings.TrimSpace(fmt.Sprint(payload["email_addr"]))
	sidToken := strings.TrimSpace(fmt.Sprint(payload["sid_token"]))
	if address == "" || address == "<nil>" {
		return "", fmt.Errorf("GuerrillaMail 创建响应缺少 email_addr")
	}
	if sidToken == "" || sidToken == "<nil>" {
		return "", fmt.Errorf("GuerrillaMail 创建响应缺少 sid_token")
	}
	s.address = address
	s.sidToken = sidToken
	log.Printf("[GuerrillaMail] 邮箱生成成功: %s", address)
	return address, nil
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *GuerrillaMailService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" || strings.TrimSpace(s.sidToken) == "" {
		return "", fmt.Errorf("GuerrillaMail 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = guerrillaMailPollInterval
	}

	log.Printf("[GuerrillaMail] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.checkEmail()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[GuerrillaMail] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if len(messages) == 0 {
			if attempt%5 == 0 {
				log.Printf("[GuerrillaMail] 暂无新邮件")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		for _, msg := range messages {
			messageID := guerrillaMailMessageID(msg)
			if messageID == "" {
				continue
			}
			if _, ok := s.checkedIDs[messageID]; ok {
				continue
			}
			s.checkedIDs[messageID] = struct{}{}

			subject := mailGWString(msg, "mail_subject", "subject", "title")
			sender := mailGWString(msg, "mail_from", "from", "sender")
			detail, err := s.fetchEmail(messageID)
			if err != nil {
				log.Printf("[GuerrillaMail] 获取邮件详情失败 (%s): %v", messageID, err)
				continue
			}
			detailText := guerrillaMailDetailText(detail)
			log.Printf("[GuerrillaMail] 发现邮件 - 发件人: %s, 主题: %s", sender, subject)
			if !mailGWLooksLikeAWSVerification(subject, sender, detailText) {
				continue
			}
			if code := mailGWCodeFromText(detailText); code != "" {
				log.Printf("[GuerrillaMail] 成功提取验证码: %s", code)
				return code, nil
			}
		}

		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *GuerrillaMailService) GetAddress() string {
	return s.address
}

func (s *GuerrillaMailService) checkEmail() ([]map[string]interface{}, error) {
	body, status, err := s.get(map[string]string{"f": "check_email", "seq": "0", "sid_token": s.sidToken})
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
	return normalizeGuerrillaMailMessages(payload), nil
}

func (s *GuerrillaMailService) fetchEmail(messageID string) (map[string]interface{}, error) {
	body, status, err := s.get(map[string]string{"f": "fetch_email", "email_id": messageID, "sid_token": s.sidToken})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("获取邮件详情 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var detail map[string]interface{}
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("解析邮件详情失败: %w", err)
	}
	return detail, nil
}

func (s *GuerrillaMailService) get(params map[string]string) ([]byte, int, error) {
	rawURL, err := guerrillaMailURL(s.apiBaseURL, params)
	if err != nil {
		return nil, 0, err
	}
	req, err := fhttp.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	httputil.SetHeaders(req, map[string]string{
		"User-Agent": guerrillaMailUserAgent,
		"Accept":     "application/json,text/plain,*/*",
		"Referer":    "https://www.guerrillamail.com/",
	})
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

func guerrillaMailURL(base string, params map[string]string) (string, error) {
	if strings.TrimSpace(base) == "" {
		base = guerrillaMailAPIBaseURL
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for key, value := range params {
		q.Set(key, value)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func normalizeGuerrillaMailMessages(payload interface{}) []map[string]interface{} {
	if m, ok := payload.(map[string]interface{}); ok {
		if list, ok := m["list"].([]interface{}); ok {
			return guerrillaMailMapItems(list)
		}
		if list, ok := m["messages"].([]interface{}); ok {
			return guerrillaMailMapItems(list)
		}
	}
	if list, ok := payload.([]interface{}); ok {
		return guerrillaMailMapItems(list)
	}
	return nil
}

func guerrillaMailMapItems(items []interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func guerrillaMailMessageID(message map[string]interface{}) string {
	for _, key := range []string{"mail_id", "id", "email_id"} {
		if text := strings.TrimSpace(fmt.Sprint(message[key])); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func guerrillaMailDetailText(detail map[string]interface{}) string {
	return strings.Join([]string{
		mailGWString(detail, "mail_subject", "subject", "title"),
		mailGWString(detail, "mail_from", "from", "sender"),
		mailGWString(detail, "mail_excerpt", "excerpt", "intro"),
		mailGWHTMLToText(mailGWString(detail, "mail_body", "body", "content")),
		mailGWHTMLToText(mailGWBodyString(detail, "body")),
	}, "\n")
}
