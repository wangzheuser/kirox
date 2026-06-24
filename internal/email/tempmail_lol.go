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

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"

	httputil "reg_go/internal/http"
	"reg_go/internal/proxy"
)

const tempMailLOLUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"

var tempMailLOLAPIBaseURL = "https://api.tempmail.lol"

// TempMailLOLService 提供 TempMail.lol 零配置临时邮箱能力。
type TempMailLOLService struct {
	client     tls_client.HttpClient
	address    string
	token      string
	checkedIDs map[string]struct{}
}

// NewTempMailLOLService 创建 TempMail.lol 临时邮箱服务。
func NewTempMailLOLService(proxyURL string) *TempMailLOLService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client, err := httputil.NewTLSClientWithTimeout(runtimeProxyURL, true, int(emailRequestTimeout/time.Second))
	if err != nil {
		log.Printf("[TempMail.lol] 邮箱代理初始化失败: %s", proxy.SanitizeError(err, runtimeProxyURL))
		client, _ = httputil.NewTLSClientWithTimeout("", true, int(emailRequestTimeout/time.Second))
	}
	return &TempMailLOLService{client: client, checkedIDs: make(map[string]struct{})}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *TempMailLOLService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[TempMail.lol] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 创建 TempMail.lol 收件箱。
func (s *TempMailLOLService) CreateWithError() (string, error) {
	body, status, err := s.get(s.apiURL("/v2/inbox/create"))
	if err != nil {
		return "", err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", fmt.Errorf("创建 TempMail.lol 邮箱 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析 TempMail.lol 创建响应失败: %w", err)
	}
	address := strings.TrimSpace(fmt.Sprint(payload["address"]))
	token := strings.TrimSpace(fmt.Sprint(payload["token"]))
	if address == "" || address == "<nil>" || token == "" || token == "<nil>" {
		return "", fmt.Errorf("TempMail.lol 创建响应缺少 address/token")
	}
	s.address = strings.ToLower(address)
	s.token = token
	log.Printf("[TempMail.lol] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *TempMailLOLService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" || strings.TrimSpace(s.token) == "" {
		return "", fmt.Errorf("TempMail.lol 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = mailGWPollInterval
	}

	log.Printf("[TempMail.lol] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		emails, expired, err := s.listEmails()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[TempMail.lol] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if expired {
			return "", fmt.Errorf("TempMail.lol 邮箱已过期")
		}
		if len(emails) == 0 {
			if attempt%5 == 0 {
				log.Printf("[TempMail.lol] 暂无新邮件")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}

		for _, msg := range emails {
			messageID := tempMailLOLMessageID(msg)
			if messageID == "" {
				messageID = fmt.Sprintf("%s|%s", mailGWString(msg, "from", "sender"), mailGWString(msg, "subject", "title"))
			}
			if _, ok := s.checkedIDs[messageID]; ok {
				continue
			}
			s.checkedIDs[messageID] = struct{}{}

			sender := mailGWString(msg, "from", "sender")
			subject := mailGWString(msg, "subject", "title")
			content := mailGWDetailText(msg)
			log.Printf("[TempMail.lol] 发现邮件 - 发件人: %s, 主题: %s", sender, subject)
			if !mailGWLooksLikeAWSVerification(subject, sender, content) {
				continue
			}
			if code := extractMailGWCode(msg); code != "" {
				log.Printf("[TempMail.lol] 成功提取验证码: %s", code)
				return code, nil
			}
		}

		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *TempMailLOLService) GetAddress() string { return s.address }

func (s *TempMailLOLService) listEmails() ([]map[string]interface{}, bool, error) {
	rawURL := s.apiURL("/v2/inbox") + "?token=" + url.QueryEscape(s.token)
	body, status, err := s.get(rawURL)
	if err != nil {
		return nil, false, err
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("获取 TempMail.lol 邮件列表 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false, fmt.Errorf("解析 TempMail.lol 邮件列表失败: %w", err)
	}
	expired, _ := payload["expired"].(bool)
	return normalizeMailGWMessages(payload), expired, nil
}

func (s *TempMailLOLService) apiURL(path string) string {
	base := strings.TrimRight(tempMailLOLAPIBaseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func (s *TempMailLOLService) get(rawURL string) ([]byte, int, error) {
	req, err := fhttp.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	httputil.SetHeaders(req, tempMailLOLHeaders())
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

func (s *TempMailLOLService) postJSON(rawURL string, payload interface{}) ([]byte, int, error) {
	reqBody, _ := json.Marshal(payload)
	req, err := fhttp.NewRequest("POST", rawURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, 0, err
	}
	headers := tempMailLOLHeaders()
	headers["Content-Type"] = "application/json"
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

func tempMailLOLHeaders() map[string]string {
	return map[string]string{
		"Accept":          "application/json, text/plain, */*",
		"User-Agent":      tempMailLOLUserAgent,
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	}
}

func tempMailLOLMessageID(message map[string]interface{}) string {
	for _, key := range []string{"id", "_id", "messageID", "uid"} {
		if text := strings.TrimSpace(fmt.Sprint(message[key])); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}
