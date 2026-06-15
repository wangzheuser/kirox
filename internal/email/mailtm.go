package email

import (
	"fmt"
	"log"
	"strings"
)

const (
	mailTMCreateRetries = 5
)

var mailTMAPIBaseURL = "https://api.mail.tm"

// MailTMService 提供 mail.tm 零配置临时邮箱能力。
// mail.tm 与 mail.gw 都采用 Hydra/JSON-LD 邮箱 API，这里复用 mail.gw 的请求、解析与验证码提取逻辑。
type MailTMService struct {
	MailGWService
}

// NewMailTMService 创建 mail.tm 临时邮箱服务。
func NewMailTMService(proxyURL string) *MailTMService {
	base := NewMailGWService(proxyURL)
	base.apiBaseURL = mailTMAPIBaseURL
	base.displayName = "mail.tm"
	return &MailTMService{MailGWService: *base}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *MailTMService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[mail.tm] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 创建 mail.tm 账号并获取访问 token。
func (s *MailTMService) CreateWithError() (string, error) {
	domains, err := s.getDomains()
	if err != nil {
		return "", err
	}
	if len(domains) == 0 {
		return "", fmt.Errorf("mail.tm 没有可用域名")
	}

	var lastErr error
	for attempt := 1; attempt <= mailTMCreateRetries; attempt++ {
		domain := domains[attempt%len(domains)]
		address := strings.ToLower(fmt.Sprintf("%s@%s", GenerateEmailName(attempt), domain))
		password := randomMailGWPassword()

		if err := s.createAccount(address, password); err != nil {
			lastErr = err
			log.Printf("[mail.tm] 创建账号失败 (%d/%d): %v", attempt, mailTMCreateRetries, err)
			continue
		}
		token, err := s.createToken(address, password)
		if err != nil {
			lastErr = err
			log.Printf("[mail.tm] 获取 token 失败 (%d/%d): %v", attempt, mailTMCreateRetries, err)
			continue
		}

		s.address = address
		s.password = password
		s.token = token
		log.Printf("[mail.tm] 邮箱生成成功: %s", address)
		return address, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("mail.tm 邮箱生成失败，已重试 %d 次: %w", mailTMCreateRetries, lastErr)
	}
	return "", fmt.Errorf("mail.tm 邮箱生成失败，已重试 %d 次", mailTMCreateRetries)
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *MailTMService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	return s.MailGWService.WaitForCode(timeoutSec, intervalSec)
}

func (s *MailTMService) getDomains() ([]string, error) {
	body, status, err := s.get(s.apiURL("/domains?page=1"), nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("获取 mail.tm 域名失败 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	return decodeMailTMDomains(body)
}

func (s *MailTMService) createAccount(address, password string) error {
	body, status, err := s.postJSON(s.apiURL("/accounts"), map[string]string{"address": address, "password": password}, "")
	if err != nil {
		return err
	}
	if status != 200 && status != 201 {
		return fmt.Errorf("创建账号 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	return nil
}

func (s *MailTMService) createToken(address, password string) (string, error) {
	body, status, err := s.postJSON(s.apiURL("/token"), map[string]string{"address": address, "password": password}, "")
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("获取 token HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	return decodeMailTMToken(body)
}
