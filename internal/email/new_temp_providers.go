package email

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"reg_go/internal/proxy"
)

const (
	mailCatchPollInterval      = 3
	tempMailoPollInterval      = 3
	generatorEmailPollInterval = 3
)

var (
	mailCatchBaseURL       = "https://mailcatch.com"
	mailCatchMessageLinkRe = regexp.MustCompile(`(?i)/api/data/[^/"']+/([^?"'/>\s]+)`)
	mailCatchDataIDRe      = regexp.MustCompile(`(?i)data-(?:id|mail-id)=["']([^"']+)["']`)

	tempMailoBaseURL = "https://tempmailo.com"
	tempMailoTokenRe = regexp.MustCompile(`(?is)name=["']__RequestVerificationToken["'][^>]*value=["']([^"']+)["']|value=["']([^"']+)["'][^>]*name=["']__RequestVerificationToken["']`)

	generatorEmailBaseURL   = "https://generator.email"
	generatorEmailAddressRe = regexp.MustCompile(`(?is)id=["']email_ch_text["'][^>]*>\s*([^<\s]+@[^<\s]+)\s*<`)
	generatorEmailDomainPRe = regexp.MustCompile(`(?is)<p[^>]+id=["']([^"']+)["'][^>]*>`)
	generatorEmailInputRe   = regexp.MustCompile(`(?is)id=["']domainName2["'][^>]*value=["']([^"']+)["']`)
	generatorEmailDomains   = []string{"gmaill.click", "mnvr.site", "shortweb.live", "email-temp.com", "jiangwy.one", "nanopools.info"}
)

type MailCatchService struct {
	client         *http.Client
	baseURL        string
	address        string
	local          string
	checkedIDs     map[string]struct{}
	localGenerator func() string
}

func NewMailCatchService(proxyURL string) *MailCatchService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	return &MailCatchService{
		client:         httpClientWithProxy(runtimeProxyURL, emailRequestTimeout),
		baseURL:        mailCatchBaseURL,
		checkedIDs:     make(map[string]struct{}),
		localGenerator: randomTempMailboxLocal,
	}
}

func (s *MailCatchService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[MailCatch] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

func (s *MailCatchService) CreateWithError() (string, error) {
	local := sanitizeMailboxLocal(s.localGenerator())
	if local == "" {
		local = sanitizeMailboxLocal(GenerateEmailName(time.Now().Nanosecond()))
	}
	if local == "" {
		return "", fmt.Errorf("MailCatch 本地邮箱名为空")
	}
	s.local = local
	s.address = strings.ToLower(local + "@mailcatch.com")
	log.Printf("[MailCatch] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

func (s *MailCatchService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.local) == "" || strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("MailCatch 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = mailCatchPollInterval
	}
	log.Printf("[MailCatch] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		listHTML, err := s.get("/api/list/" + url.PathEscape(s.local))
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[MailCatch] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if code := codeFromProviderText(listHTML); code != "" {
			log.Printf("[MailCatch] 成功提取验证码: %s", code)
			return code, nil
		}
		ids := extractMailCatchMessageIDs(listHTML)
		if len(ids) == 0 {
			if attempt%5 == 0 {
				log.Printf("[MailCatch] 暂无新邮件")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		for _, id := range ids {
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			detail, err := s.get("/api/data/" + url.PathEscape(s.local) + "/" + url.PathEscape(id) + "?show_images=1")
			if err != nil {
				log.Printf("[MailCatch] 获取邮件详情失败: %v", err)
				continue
			}
			if code := codeFromProviderText(detail); code != "" {
				log.Printf("[MailCatch] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

func (s *MailCatchService) GetAddress() string { return s.address }

func (s *MailCatchService) get(path string) (string, error) {
	rawURL := strings.TrimRight(s.baseURL, "/") + path
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain,*/*")
	req.Header.Set("Referer", strings.TrimRight(s.baseURL, "/")+"/en/temporary-inbox?box="+url.QueryEscape(s.local))
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
		return "", fmt.Errorf("MailCatch HTTP %d: %s", resp.StatusCode, shortMailGWBody(string(body), 200))
	}
	return string(body), nil
}

type TempMailoService struct {
	client       *http.Client
	directClient *http.Client
	baseURL      string
	token        string
	address      string
}

func NewTempMailoService(proxyURL string) *TempMailoService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	jar, _ := cookiejar.New(nil)
	client := httpClientWithProxy(runtimeProxyURL, emailRequestTimeout)
	client.Jar = jar
	directClient := httpClientWithProxy("", emailRequestTimeout)
	directClient.Jar = jar
	return &TempMailoService{client: client, directClient: directClient, baseURL: tempMailoBaseURL}
}

func (s *TempMailoService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[TempMailo] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

func (s *TempMailoService) CreateWithError() (string, error) {
	html, status, err := s.request(http.MethodGet, "/", nil, false)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("TempMailo 首页 HTTP %d: %s", status, shortMailGWBody(html, 200))
	}
	token := extractTempMailoToken(html)
	if token == "" {
		return "", fmt.Errorf("TempMailo 首页未返回 antiforgery token")
	}
	s.token = token
	path := "/changemail?_r=" + url.QueryEscape(randomDigits(12))
	body, status, err := s.request(http.MethodGet, path, nil, true)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("TempMailo changemail HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	address := strings.ToLower(strings.TrimSpace(body))
	address = strings.Trim(address, `"' \r\n\t`)
	if !strings.Contains(address, "@") {
		return "", fmt.Errorf("TempMailo 未返回邮箱地址: %s", shortMailGWBody(body, 120))
	}
	s.address = address
	log.Printf("[TempMailo] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

func (s *TempMailoService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" || strings.TrimSpace(s.token) == "" {
		return "", fmt.Errorf("TempMailo 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = tempMailoPollInterval
	}
	log.Printf("[TempMailo] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	payload := map[string]string{"mail": s.address}
	for time.Now().Before(deadline) {
		attempt++
		body, status, err := s.request(http.MethodPost, "/", payload, true)
		if err != nil || status != http.StatusOK {
			if attempt%5 == 0 {
				log.Printf("[TempMailo] 获取邮件列表失败: status=%d err=%v body=%s", status, err, shortMailGWBody(body, 160))
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if code := codeFromProviderText(normalizeTempMailoInboxText(body)); code != "" {
			log.Printf("[TempMailo] 成功提取验证码: %s", code)
			return code, nil
		}
		if attempt%5 == 0 {
			log.Printf("[TempMailo] 暂无新邮件")
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

func (s *TempMailoService) GetAddress() string { return s.address }

func (s *TempMailoService) request(method, path string, payload interface{}, token bool) (string, int, error) {
	body, status, err := s.doRequest(s.client, method, path, payload, token)
	if status == http.StatusForbidden && tempMailoLooksLikeCloudflare(body) && s.directClient != nil {
		log.Printf("[TempMailo] 邮箱代理触发 Cloudflare 403，回退直连邮箱 API")
		return s.doRequest(s.directClient, method, path, payload, token)
	}
	return body, status, err
}

func (s *TempMailoService) doRequest(client *http.Client, method, path string, payload interface{}, token bool) (string, int, error) {
	rawURL := strings.TrimRight(s.baseURL, "/") + path
	var body io.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	if method == http.MethodGet && path == "/" && payload == nil && !token {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	} else {
		req.Header.Set("Accept", "application/json,text/plain,*/*")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}
	req.Header.Set("Referer", strings.TrimRight(s.baseURL, "/")+"/")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token && s.token != "" {
		req.Header.Set("RequestVerificationToken", s.token)
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

func tempMailoLooksLikeCloudflare(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "just a moment") || strings.Contains(lower, "cloudflare") || strings.Contains(lower, "cf-ray")
}

type GeneratorEmailService struct {
	client         *http.Client
	baseURL        string
	address        string
	local          string
	domain         string
	domains        []string
	localGenerator func() string
}

func NewGeneratorEmailService(proxyURL string) *GeneratorEmailService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	jar, _ := cookiejar.New(nil)
	client := httpClientWithProxy(runtimeProxyURL, emailRequestTimeout)
	client.Jar = jar
	return &GeneratorEmailService{
		client:         client,
		baseURL:        generatorEmailBaseURL,
		domains:        append([]string{}, generatorEmailDomains...),
		localGenerator: randomTempMailboxLocal,
	}
}

func (s *GeneratorEmailService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[Generator.Email] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

func (s *GeneratorEmailService) CreateWithError() (string, error) {
	local := sanitizeMailboxLocal(s.localGenerator())
	if local == "" {
		local = sanitizeMailboxLocal(GenerateEmailName(time.Now().Nanosecond()))
	}
	if local == "" {
		return "", fmt.Errorf("Generator.Email 本地邮箱名为空")
	}
	domains := appendUniqueDomains(nil, s.discoverDomains()...)
	domains = appendUniqueDomains(domains, s.domains...)
	if len(domains) == 0 {
		domains = appendUniqueDomains(domains, generatorEmailDomains...)
	}
	var lastErr error
	for _, domain := range domains {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			continue
		}
		if ok, err := s.validateAddress(local, domain); err != nil {
			lastErr = err
			continue
		} else if !ok {
			lastErr = fmt.Errorf("Generator.Email 域名不可用: %s", domain)
			continue
		}
		body, status, err := s.get("/" + url.PathEscape(local+"@"+domain))
		if err != nil {
			lastErr = err
			continue
		}
		if status != http.StatusOK {
			lastErr = fmt.Errorf("Generator.Email 创建邮箱 HTTP %d: %s", status, shortMailGWBody(body, 200))
			continue
		}
		address := strings.ToLower(strings.TrimSpace(extractGeneratorEmailAddress(body)))
		if address == "" {
			lastErr = fmt.Errorf("Generator.Email 页面未返回邮箱地址")
			continue
		}
		s.local = local
		s.domain = domain
		s.address = address
		log.Printf("[Generator.Email] 邮箱生成成功: %s", s.address)
		return s.address, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("Generator.Email 未配置可用域名")
}

func (s *GeneratorEmailService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" || strings.TrimSpace(s.local) == "" || strings.TrimSpace(s.domain) == "" {
		return "", fmt.Errorf("Generator.Email 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = generatorEmailPollInterval
	}
	log.Printf("[Generator.Email] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	path := "/" + url.PathEscape(s.domain) + "/" + url.PathEscape(s.local)
	for time.Now().Before(deadline) {
		attempt++
		body, status, err := s.get(path)
		if err != nil || status != http.StatusOK {
			if attempt%5 == 0 {
				log.Printf("[Generator.Email] 获取收件箱失败: status=%d err=%v body=%s", status, err, shortMailGWBody(body, 160))
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if code := codeFromProviderText(body); code != "" {
			log.Printf("[Generator.Email] 成功提取验证码: %s", code)
			return code, nil
		}
		if attempt%5 == 0 {
			log.Printf("[Generator.Email] 暂无新邮件")
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

func (s *GeneratorEmailService) GetAddress() string { return s.address }

func (s *GeneratorEmailService) discoverDomains() []string {
	body, status, err := s.get("/")
	if err != nil || status != http.StatusOK {
		if err != nil {
			log.Printf("[Generator.Email] 动态获取域名失败，使用内置域名: %v", err)
		} else {
			log.Printf("[Generator.Email] 动态获取域名 HTTP %d，使用内置域名", status)
		}
		return nil
	}
	domains := extractGeneratorEmailDomains(body)
	if len(domains) > 0 {
		log.Printf("[Generator.Email] 动态获取到 %d 个域名", len(domains))
	}
	return domains
}

func (s *GeneratorEmailService) validateAddress(local, domain string) (bool, error) {
	form := url.Values{}
	form.Set("usr", local)
	form.Set("dmn", domain)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(s.baseURL, "/")+"/check_adres_validation3.php", strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Referer", strings.TrimRight(s.baseURL, "/")+"/")
	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	if resp.StatusCode/100 != 2 {
		return false, fmt.Errorf("Generator.Email 校验 HTTP %d: %s", resp.StatusCode, shortMailGWBody(string(body), 200))
	}
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return strings.Contains(strings.ToLower(string(body)), `"status": "good"`) || strings.Contains(strings.ToLower(string(body)), `"status":"good"`), nil
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(data["status"])), "good"), nil
}

func (s *GeneratorEmailService) get(path string) (string, int, error) {
	rawURL := strings.TrimRight(s.baseURL, "/") + path
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Referer", strings.TrimRight(s.baseURL, "/")+"/")
	if s.domain != "" && s.local != "" {
		req.Header.Set("Cookie", "surl="+url.QueryEscape(s.domain+"/"+s.local))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}
	return string(body), resp.StatusCode, nil
}

func extractMailCatchMessageIDs(html string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if decoded, err := url.PathUnescape(id); err == nil {
			id = decoded
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, match := range mailCatchMessageLinkRe.FindAllStringSubmatch(html, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}
	for _, match := range mailCatchDataIDRe.FindAllStringSubmatch(html, -1) {
		if len(match) > 1 {
			add(match[1])
		}
	}
	return out
}

func extractTempMailoToken(html string) string {
	match := tempMailoTokenRe.FindStringSubmatch(html)
	for i := 1; i < len(match); i++ {
		if strings.TrimSpace(match[i]) != "" {
			return strings.TrimSpace(match[i])
		}
	}
	return ""
}

func normalizeTempMailoInboxText(body string) string {
	var data interface{}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return body
	}
	var b strings.Builder
	appendValueText(&b, data)
	return b.String()
}

func appendValueText(b *strings.Builder, value interface{}) {
	switch v := value.(type) {
	case string:
		b.WriteString(v)
		b.WriteByte('\n')
	case []interface{}:
		for _, item := range v {
			appendValueText(b, item)
		}
	case map[string]interface{}:
		for _, item := range v {
			appendValueText(b, item)
		}
	}
}

func extractGeneratorEmailAddress(html string) string {
	match := generatorEmailAddressRe.FindStringSubmatch(html)
	if len(match) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}

func extractGeneratorEmailDomains(html string) []string {
	domains := []string{}
	for _, match := range generatorEmailInputRe.FindAllStringSubmatch(html, -1) {
		if len(match) > 1 {
			domains = appendUniqueDomains(domains, match[1])
		}
	}
	for _, match := range generatorEmailDomainPRe.FindAllStringSubmatch(html, -1) {
		if len(match) > 1 {
			domains = appendUniqueDomains(domains, match[1])
		}
	}
	return domains
}

func codeFromProviderText(text string) string {
	cleaned := mailGWHTMLToText(text)
	if !mailGWLooksLikeAWSVerification("", "", cleaned) {
		return ""
	}
	return mailGWCodeFromText(cleaned)
}

func sanitizeMailboxLocal(local string) string {
	local = strings.ToLower(strings.TrimSpace(local))
	var b strings.Builder
	for _, r := range local {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), ".-_")
}

func randomTempMailboxLocal() string {
	return "kiro" + randomAlphaNumLower(12)
}

func randomAlphaNumLower(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	var b strings.Builder
	for i := 0; i < n; i++ {
		idx := 0
		if v, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(alphabet)))); err == nil {
			idx = int(v.Int64())
		} else {
			idx = int(time.Now().UnixNano()) % len(alphabet)
		}
		b.WriteByte(alphabet[idx])
	}
	return b.String()
}

func randomDigits(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		digit := 0
		if v, err := cryptorand.Int(cryptorand.Reader, big.NewInt(10)); err == nil {
			digit = int(v.Int64())
		} else {
			digit = int(time.Now().UnixNano() % 10)
		}
		b.WriteByte(byte('0' + digit))
	}
	return b.String()
}
