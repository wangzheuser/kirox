package email

import (
	"bytes"
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

const sessionTempSitePollInterval = 3

var sessionTempSiteCSRFRe = regexp.MustCompile(`const\s+CSRF\s*=\s*"([^"]+)"`)

type SessionTempSiteService struct {
	client     *http.Client
	baseURL    string
	label      string
	address    string
	checkedIDs map[string]struct{}
}

func NewMinuteInboxService(proxyURL string) *SessionTempSiteService {
	return newSessionTempSiteService(proxyURL, "https://www.minuteinbox.com", "MinuteInbox")
}

func newSessionTempSiteService(proxyURL, baseURL, label string) *SessionTempSiteService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	jar, _ := cookiejar.New(nil)
	client := httpClientWithProxy(runtimeProxyURL, emailRequestTimeout)
	client.Jar = jar
	return &SessionTempSiteService{
		client:     client,
		baseURL:    strings.TrimRight(baseURL, "/"),
		label:      label,
		checkedIDs: make(map[string]struct{}),
	}
}

func (s *SessionTempSiteService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[%s] 创建邮箱失败: %v", s.label, err)
		return ""
	}
	return address
}

func (s *SessionTempSiteService) CreateWithError() (string, error) {
	home, status, err := s.request(http.MethodGet, "/", nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("%s 首页 HTTP %d: %s", s.label, status, shortMailGWBody(home, 200))
	}
	csrf := extractSessionTempSiteCSRF(home)
	if csrf == "" {
		return "", fmt.Errorf("%s 首页未返回 CSRF", s.label)
	}
	body, status, err := s.request(http.MethodGet, "/index/index?csrf_token="+url.QueryEscape(csrf), nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("%s 创建邮箱 HTTP %d: %s", s.label, status, shortMailGWBody(body, 200))
	}
	address := extractSessionTempSiteAddress(body)
	if address == "" {
		return "", fmt.Errorf("%s 未返回邮箱地址: %s", s.label, shortMailGWBody(body, 200))
	}
	s.address = address
	log.Printf("[%s] 邮箱生成成功: %s", s.label, s.address)
	return s.address, nil
}

func (s *SessionTempSiteService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("%s 邮箱未创建", s.label)
	}
	if intervalSec <= 0 {
		intervalSec = sessionTempSitePollInterval
	}
	log.Printf("[%s] 开始等待验证码: %s", s.label, s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.refresh()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[%s] 获取邮件列表失败: %v", s.label, err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		for _, msg := range messages {
			id := strings.TrimSpace(fmt.Sprint(msg["id"]))
			if id == "" {
				continue
			}
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			detail, err := s.detail(id)
			if err != nil {
				log.Printf("[%s] 获取邮件详情失败: %v", s.label, err)
				continue
			}
			var b strings.Builder
			for _, key := range []string{"od", "predmet", "telo", "body", "html"} {
				if v, ok := detail[key]; ok {
					b.WriteString(fmt.Sprint(v))
					b.WriteByte('\n')
				}
			}
			if code := codeFromProviderText(b.String()); code != "" {
				log.Printf("[%s] 成功提取验证码: %s", s.label, code)
				return code, nil
			}
		}
		if attempt%5 == 0 {
			log.Printf("[%s] 暂无新邮件", s.label)
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

func (s *SessionTempSiteService) GetAddress() string { return s.address }

func (s *SessionTempSiteService) refresh() ([]map[string]interface{}, error) {
	body, status, err := s.request(http.MethodGet, "/index/refresh", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s 列表 HTTP %d: %s", s.label, status, shortMailGWBody(body, 200))
	}
	body = strings.TrimSpace(strings.TrimPrefix(body, "\ufeff"))
	if body == "" || body == "0" || body == "false" {
		return nil, nil
	}
	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(body), &messages); err != nil {
		return nil, fmt.Errorf("%s 列表 JSON 解析失败: %w body=%s", s.label, err, shortMailGWBody(body, 200))
	}
	return messages, nil
}

func (s *SessionTempSiteService) detail(id string) (map[string]interface{}, error) {
	form := url.Values{}
	form.Set("id", id)
	body, status, err := s.request(http.MethodPost, "/index/email", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("%s 详情 HTTP %d: %s", s.label, status, shortMailGWBody(body, 200))
	}
	body = strings.TrimSpace(strings.TrimPrefix(body, "\ufeff"))
	var detail map[string]interface{}
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		return nil, fmt.Errorf("%s 详情 JSON 解析失败: %w body=%s", s.label, err, shortMailGWBody(body, 200))
	}
	return detail, nil
}

func (s *SessionTempSiteService) request(method, path string, body io.Reader) (string, int, error) {
	rawURL := s.baseURL + path
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	if method == http.MethodGet && path == "/" {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	} else {
		req.Header.Set("Accept", "application/json,text/javascript,text/html,*/*")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}
	req.Header.Set("Referer", s.baseURL+"/")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})), resp.StatusCode, nil
}

func extractSessionTempSiteCSRF(html string) string {
	match := sessionTempSiteCSRFRe.FindStringSubmatch(html)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func extractSessionTempSiteAddress(body string) string {
	body = strings.TrimSpace(strings.TrimPrefix(body, "\ufeff"))
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(body), &data); err == nil {
		if address := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["email"]))); strings.Contains(address, "@") {
			return address
		}
	}
	return ""
}
