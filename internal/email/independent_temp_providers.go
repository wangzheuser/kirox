package email

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"reg_go/internal/proxy"
)

const independentTempPollInterval = 3

var (
	smailProAppBaseURL = "https://smailpro.com"
	smailProAPIBaseURL = "https://api.sonjj.com/v1/temp_email"

	tempMailboxBaseURL     = "https://tempmailbox.net"
	tempMailboxCSRFTokenRe = regexp.MustCompile(`(?is)<meta\s+name=["']csrf-token["']\s+content=["']([^"']+)["']`)
)

type SmailProService struct {
	client     *http.Client
	appBaseURL string
	apiBaseURL string
	address    string
	checkedIDs map[string]struct{}
}

func NewSmailProService(proxyURL string) *SmailProService {
	return &SmailProService{
		client:     newIndependentTempHTTPClient(proxyURL),
		appBaseURL: smailProAppBaseURL,
		apiBaseURL: smailProAPIBaseURL,
		checkedIDs: make(map[string]struct{}),
	}
}

func (s *SmailProService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[SmailPro] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

func (s *SmailProService) CreateWithError() (string, error) {
	payload, err := s.payload(map[string]string{"url": strings.TrimRight(s.apiBaseURL, "/") + "/create"})
	if err != nil {
		return "", err
	}
	body, status, err := s.getAPI("/create", map[string]string{"payload": payload})
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("SmailPro 创建邮箱 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	var data struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return "", fmt.Errorf("SmailPro 创建邮箱 JSON 解析失败: %w body=%s", err, shortMailGWBody(body, 200))
	}
	if !strings.Contains(data.Email, "@") {
		return "", fmt.Errorf("SmailPro 未返回邮箱地址: %s", shortMailGWBody(body, 200))
	}
	s.address = strings.ToLower(strings.TrimSpace(data.Email))
	log.Printf("[SmailPro] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

func (s *SmailProService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("SmailPro 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = independentTempPollInterval
	}
	log.Printf("[SmailPro] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		text, ids, err := s.inboxTextAndIDs()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[SmailPro] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if code := codeFromProviderText(text); code != "" {
			log.Printf("[SmailPro] 成功提取验证码: %s", code)
			return code, nil
		}
		for _, id := range ids {
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			detail, err := s.messageText(id)
			if err != nil {
				log.Printf("[SmailPro] 获取邮件详情失败: %v", err)
				continue
			}
			if code := codeFromProviderText(detail); code != "" {
				log.Printf("[SmailPro] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		if attempt%5 == 0 {
			log.Printf("[SmailPro] 暂无新邮件")
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

func (s *SmailProService) GetAddress() string { return s.address }

func (s *SmailProService) inboxTextAndIDs() (string, []string, error) {
	payload, err := s.payload(map[string]string{"url": strings.TrimRight(s.apiBaseURL, "/") + "/inbox", "email": s.address})
	if err != nil {
		return "", nil, err
	}
	body, status, err := s.getAPI("/inbox", map[string]string{"payload": payload})
	if err != nil {
		return "", nil, err
	}
	if status != http.StatusOK {
		return "", nil, fmt.Errorf("SmailPro 列表 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	var data struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return body, nil, nil
	}
	var b strings.Builder
	ids := []string{}
	for _, msg := range data.Messages {
		appendValueText(&b, msg)
		id := strings.TrimSpace(fmt.Sprint(msg["mid"]))
		if id == "" {
			id = strings.TrimSpace(fmt.Sprint(msg["id"]))
		}
		if id != "" {
			ids = append(ids, id)
		}
	}
	return b.String(), ids, nil
}

func (s *SmailProService) messageText(id string) (string, error) {
	payload, err := s.payload(map[string]string{"url": strings.TrimRight(s.apiBaseURL, "/") + "/message", "email": s.address, "mid": id})
	if err != nil {
		return "", err
	}
	body, status, err := s.getAPI("/message", map[string]string{"payload": payload})
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("SmailPro 详情 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	var data interface{}
	var b strings.Builder
	if err := json.Unmarshal([]byte(body), &data); err == nil {
		appendValueText(&b, data)
		return b.String(), nil
	}
	return body, nil
}

func (s *SmailProService) payload(params map[string]string) (string, error) {
	body, status, err := independentTempRequest(s.client, http.MethodGet, s.appBaseURL, "/app/payload", params, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("SmailPro payload HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	payload := strings.TrimSpace(body)
	if payload == "" {
		return "", fmt.Errorf("SmailPro payload 为空")
	}
	return payload, nil
}

func (s *SmailProService) getAPI(path string, params map[string]string) (string, int, error) {
	return independentTempRequest(s.client, http.MethodGet, s.apiBaseURL, path, params, nil)
}

type TempMailboxService struct {
	client     *http.Client
	baseURL    string
	csrf       string
	address    string
	checkedIDs map[string]struct{}
}

func NewTempMailboxService(proxyURL string) *TempMailboxService {
	return &TempMailboxService{
		client:     newIndependentTempHTTPClient(proxyURL),
		baseURL:    tempMailboxBaseURL,
		checkedIDs: make(map[string]struct{}),
	}
}

func (s *TempMailboxService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[TempMailbox] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

func (s *TempMailboxService) CreateWithError() (string, error) {
	home, status, err := independentTempRequest(s.client, http.MethodGet, s.baseURL, "/", nil, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("TempMailbox 首页 HTTP %d: %s", status, shortMailGWBody(home, 200))
	}
	match := tempMailboxCSRFTokenRe.FindStringSubmatch(home)
	if len(match) < 2 || strings.TrimSpace(match[1]) == "" {
		return "", fmt.Errorf("TempMailbox 首页未返回 CSRF")
	}
	s.csrf = strings.TrimSpace(match[1])
	text, _, err := s.messages()
	if err != nil {
		return "", err
	}
	var data struct {
		Mailbox string `json:"mailbox"`
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		return "", fmt.Errorf("TempMailbox 创建邮箱 JSON 解析失败: %w body=%s", err, shortMailGWBody(text, 200))
	}
	if !strings.Contains(data.Mailbox, "@") {
		return "", fmt.Errorf("TempMailbox 未返回邮箱地址: %s", shortMailGWBody(text, 200))
	}
	s.address = strings.ToLower(strings.TrimSpace(data.Mailbox))
	log.Printf("[TempMailbox] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

func (s *TempMailboxService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("TempMailbox 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = independentTempPollInterval
	}
	log.Printf("[TempMailbox] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		text, ids, err := s.messages()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[TempMailbox] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if code := codeFromProviderText(text); code != "" {
			log.Printf("[TempMailbox] 成功提取验证码: %s", code)
			return code, nil
		}
		for _, id := range ids {
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			detail, err := s.viewMessage(id)
			if err != nil {
				log.Printf("[TempMailbox] 获取邮件详情失败: %v", err)
				continue
			}
			if code := codeFromProviderText(detail); code != "" {
				log.Printf("[TempMailbox] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		if attempt%5 == 0 {
			log.Printf("[TempMailbox] 暂无新邮件")
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

func (s *TempMailboxService) GetAddress() string { return s.address }

func (s *TempMailboxService) messages() (string, []string, error) {
	form := url.Values{}
	form.Set("_token", s.csrf)
	form.Set("captcha", "")
	body, status, err := independentTempRequest(s.client, http.MethodPost, s.baseURL, "/get_messages", nil, strings.NewReader(form.Encode()))
	if err != nil {
		return "", nil, err
	}
	if status != http.StatusOK {
		return "", nil, fmt.Errorf("TempMailbox 列表 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	var data struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	ids := []string{}
	if err := json.Unmarshal([]byte(body), &data); err == nil {
		for _, msg := range data.Messages {
			id := strings.TrimSpace(fmt.Sprint(msg["id"]))
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return body, ids, nil
}

func (s *TempMailboxService) viewMessage(id string) (string, error) {
	body, status, err := independentTempRequest(s.client, http.MethodGet, s.baseURL, "/view/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("TempMailbox 详情 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	return body, nil
}

func newIndependentTempHTTPClient(proxyURL string) *http.Client {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client := httpClientWithProxy(runtimeProxyURL, emailRequestTimeout)
	jar, _ := cookiejar.New(nil)
	client.Jar = jar
	return client
}

func independentTempRequest(client *http.Client, method, baseURL, path string, params map[string]string, body io.Reader) (string, int, error) {
	rawURL := strings.TrimRight(baseURL, "/") + path
	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		sep := "?"
		if strings.Contains(rawURL, "?") {
			sep = "&"
		}
		rawURL += sep + values.Encode()
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "application/json,text/html,application/xhtml+xml,text/plain,*/*")
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/")+"/")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(respBody), resp.StatusCode, nil
}
