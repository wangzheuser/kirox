package email

import (
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

const (
	mailTempPollInterval = 3
	mailTempUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
)

var (
	mailTempBaseURL    = "https://mail-temp.com"
	mailTempAddressRe  = regexp.MustCompile(`(?is)id=["']email_ch_text["'][^>]*>\s*([^<\s]+@[^<\s]+)\s*<`)
	mailTempMessageRe  = regexp.MustCompile(`(?i)href=["']([^"']+/[a-z0-9.-]+/[a-z0-9._-]+/[^"']+)["']`)
	mailTempEmailClean = regexp.MustCompile(`[^a-zA-Z0-9._%+\-@]`)
)

// MailTempService 提供 mail-temp.com 零配置临时邮箱能力。
type MailTempService struct {
	client     *http.Client
	baseURL    string
	address    string
	checkedURL map[string]struct{}
}

// NewMailTempService 创建 mail-temp.com 临时邮箱服务。
func NewMailTempService(proxyURL string) *MailTempService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client := httpClientWithProxy(runtimeProxyURL, emailRequestTimeout)
	return &MailTempService{client: client, baseURL: mailTempBaseURL, checkedURL: make(map[string]struct{})}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *MailTempService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[MailTemp] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 打开首页并解析页面生成的临时邮箱。
func (s *MailTempService) CreateWithError() (string, error) {
	body, status, err := s.get("/")
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("创建 MailTemp 邮箱 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	address := extractMailTempAddress(string(body))
	if address == "" {
		return "", fmt.Errorf("MailTemp 首页未返回邮箱地址")
	}
	s.address = address
	log.Printf("[MailTemp] 邮箱生成成功: %s", address)
	return address, nil
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *MailTempService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("MailTemp 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = mailTempPollInterval
	}

	log.Printf("[MailTemp] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		html, err := s.fetchMailboxHTML()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[MailTemp] 获取收件箱失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if code := mailTempCodeFromHTML(html); code != "" {
			log.Printf("[MailTemp] 成功提取验证码: %s", code)
			return code, nil
		}
		for _, link := range extractMailTempMessageLinks(html) {
			if _, ok := s.checkedURL[link]; ok {
				continue
			}
			s.checkedURL[link] = struct{}{}
			body, status, err := s.get(link)
			if err != nil || status != http.StatusOK {
				continue
			}
			text := string(body)
			if code := mailTempCodeFromHTML(text); code != "" {
				log.Printf("[MailTemp] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *MailTempService) GetAddress() string {
	return s.address
}

func (s *MailTempService) fetchMailboxHTML() (string, error) {
	login, domain := splitMailTempAddress(s.address)
	if login == "" || domain == "" {
		return "", fmt.Errorf("MailTemp 地址格式异常: %s", s.address)
	}
	body, status, err := s.get("/" + domain + "/" + login)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("获取收件箱 HTTP %d: %s", status, shortMailGWBody(string(body), 200))
	}
	return string(body), nil
}

func (s *MailTempService) get(pathOrURL string) ([]byte, int, error) {
	rawURL, err := mailTempURL(s.baseURL, pathOrURL)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", strings.TrimRight(s.baseURL, "/")+"/")
	if cookie := s.mailboxCookie(); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
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

func mailTempURL(base, pathOrURL string) (string, error) {
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		return pathOrURL, nil
	}
	if strings.TrimSpace(base) == "" {
		base = mailTempBaseURL
	}
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(pathOrURL, "/") {
		pathOrURL = "/" + pathOrURL
	}
	u.Path = strings.TrimRight(u.Path, "/") + pathOrURL
	return u.String(), nil
}

func extractMailTempAddress(html string) string {
	match := mailTempAddressRe.FindStringSubmatch(html)
	if len(match) < 2 {
		return ""
	}
	address := strings.ToLower(strings.TrimSpace(mailTempEmailClean.ReplaceAllString(match[1], "")))
	if !strings.Contains(address, "@") {
		return ""
	}
	return address
}

func splitMailTempAddress(address string) (string, string) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(address)), "@")
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

func extractMailTempMessageLinks(html string) []string {
	matches := mailTempMessageRe.FindAllStringSubmatch(html, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		link := strings.TrimSpace(match[1])
		if link == "" {
			continue
		}
		if _, ok := seen[link]; ok {
			continue
		}
		seen[link] = struct{}{}
		out = append(out, link)
	}
	return out
}

func mailTempCodeFromHTML(html string) string {
	if !mailGWLooksLikeAWSVerification("", "", html) {
		return ""
	}
	return mailGWCodeFromText(html)
}

func (s *MailTempService) mailboxCookie() string {
	login, domain := splitMailTempAddress(s.address)
	if login == "" || domain == "" {
		return ""
	}
	return "surl=" + domain + "/" + login
}
