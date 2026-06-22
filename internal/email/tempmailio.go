package email

import (
	"bytes"
	cryptorand "crypto/rand"
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

var tempMailIODomains = []string{
	"bltiwd.com",
	"wnbaldwy.com",
	"bwmyga.com",
	"ozsaip.com",
	"yzcalo.com",
	"lnovic.com",
	"ruutukf.com",
	"gmeenramy.com",
}

// TempMailIOService 提供 temp-mail.io 公共 API 临时邮箱能力。
type TempMailIOService struct {
	client        *http.Client
	baseURL       string
	address       string
	token         string
	domain        string
	checkedIDs    map[string]struct{}
	nameGenerator func() string
}

type tempMailIONewEmailResponse struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

type tempMailIOMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	FromEmail string `json:"from_email"`
	Subject   string `json:"subject"`
	BodyText  string `json:"body_text"`
	BodyHTML  string `json:"body_html"`
	Text      string `json:"text"`
	HTML      string `json:"html"`
	Body      string `json:"body"`
}

// NewTempMailIOService 创建固定域名的 temp-mail.io 临时邮箱服务。
func NewTempMailIOService(proxyURL string, domain string) *TempMailIOService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	return &TempMailIOService{
		client:        httpClientWithProxy(runtimeProxyURL, emailRequestTimeout),
		baseURL:       "https://api.internal.temp-mail.io/api/v3",
		domain:        strings.ToLower(strings.TrimSpace(domain)),
		checkedIDs:    make(map[string]struct{}),
		nameGenerator: randomTempMailIOName,
	}
}

// NewTempMailIOBltiwdService 创建固定使用 bltiwd.com 域名的 temp-mail.io 服务。
func NewTempMailIOBltiwdService(proxyURL string) *TempMailIOService {
	return NewTempMailIOService(proxyURL, "bltiwd.com")
}

// NewTempMailIOWnbaldwyService 创建固定使用 wnbaldwy.com 域名的 temp-mail.io 服务。
func NewTempMailIOWnbaldwyService(proxyURL string) *TempMailIOService {
	return NewTempMailIOService(proxyURL, "wnbaldwy.com")
}

// NewTempMailIOBwmygaService 创建固定使用 bwmyga.com 域名的 temp-mail.io 服务。
func NewTempMailIOBwmygaService(proxyURL string) *TempMailIOService {
	return NewTempMailIOService(proxyURL, "bwmyga.com")
}

// NewTempMailIOOzsaipService 创建固定使用 ozsaip.com 域名的 temp-mail.io 服务。
func NewTempMailIOOzsaipService(proxyURL string) *TempMailIOService {
	return NewTempMailIOService(proxyURL, "ozsaip.com")
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *TempMailIOService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[TempMailIO] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 创建固定域名的 temp-mail.io 临时邮箱。
func (s *TempMailIOService) CreateWithError() (string, error) {
	domain := strings.ToLower(strings.TrimSpace(s.domain))
	if domain == "" {
		domain = tempMailIODomains[0]
	}
	name := strings.ToLower(strings.TrimSpace(s.nameGenerator()))
	if name == "" {
		name = randomTempMailIOName()
	}
	payload, _ := json.Marshal(map[string]string{"name": name, "domain": domain})
	req, err := http.NewRequest("POST", strings.TrimRight(s.baseURL, "/")+"/email/new", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://temp-mail.io")
	req.Header.Set("Referer", "https://temp-mail.io/")
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("TempMailIO 创建邮箱 HTTP %d: %s", resp.StatusCode, shortMailGWBody(string(body), 300))
	}
	var result tempMailIONewEmailResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析 TempMailIO 创建邮箱响应失败: %w", err)
	}
	address := strings.ToLower(strings.TrimSpace(result.Email))
	if address == "" {
		return "", fmt.Errorf("TempMailIO 未返回邮箱地址")
	}
	s.address = address
	s.token = strings.TrimSpace(result.Token)
	log.Printf("[TempMailIO] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *TempMailIOService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("TempMailIO 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = 3
	}
	log.Printf("[TempMailIO] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.listMessages()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[TempMailIO] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		for _, msg := range messages {
			id := strings.TrimSpace(msg.ID)
			if id == "" {
				id = fmt.Sprintf("%s|%s|%s", msg.From, msg.FromEmail, msg.Subject)
			}
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			sender := firstDropMailText(msg.FromEmail, msg.From)
			combined := strings.Join([]string{msg.Subject, sender, msg.BodyText, msg.Text, mailGWHTMLToText(msg.BodyHTML), mailGWHTMLToText(msg.HTML), msg.Body}, "\n")
			if !mailGWLooksLikeAWSVerification(msg.Subject, sender, combined) {
				continue
			}
			if code := dropMailCodeFromText(combined); code != "" {
				log.Printf("[TempMailIO] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *TempMailIOService) GetAddress() string { return s.address }

func (s *TempMailIOService) listMessages() ([]tempMailIOMessage, error) {
	endpoint := strings.TrimRight(s.baseURL, "/") + "/email/" + url.PathEscape(s.address) + "/messages"
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://temp-mail.io/")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("TempMailIO 获取邮件 HTTP %d: %s", resp.StatusCode, shortMailGWBody(string(body), 300))
	}
	var messages []tempMailIOMessage
	if err := json.Unmarshal(body, &messages); err != nil {
		return nil, fmt.Errorf("解析 TempMailIO 邮件列表失败: %w", err)
	}
	return messages, nil
}

func randomTempMailIOName() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	raw := make([]byte, 12)
	if _, err := cryptorand.Read(raw); err != nil {
		return fmt.Sprintf("kiro%x", time.Now().UnixNano())
	}
	var b strings.Builder
	for _, v := range raw {
		b.WriteByte(alphabet[int(v)%len(alphabet)])
	}
	return b.String()
}
