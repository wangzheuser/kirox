package email

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	stdhtml "html"
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

	goneBoxAPIBaseURL   = "https://api.gonebox.email/api/v1"
	openInboxAPIBaseURL = "https://api.openinbox.io/api"
	blinkBoxBaseURL     = "https://blinkboxapp.com"
	goneBoxDomain       = "gonebox.email"
	blinkBoxCSRFRe      = regexp.MustCompile(`(?is)data-csrf=["']([^"']+)["']`)
	blinkBoxSnapshotRe  = regexp.MustCompile(`(?is)wire:snapshot=["']([^"']+)["']`)
	blinkBoxTagRe       = regexp.MustCompile(`(?is)<[^>]+>`)
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

type GoneBoxService struct {
	client     *http.Client
	apiBaseURL string
	domain     string
	address    string
	checkedIDs map[string]struct{}
}

func NewGoneBoxService(proxyURL string) *GoneBoxService {
	return &GoneBoxService{
		client:     newIndependentTempHTTPClient(proxyURL),
		apiBaseURL: goneBoxAPIBaseURL,
		domain:     goneBoxDomain,
		checkedIDs: make(map[string]struct{}),
	}
}

func (s *GoneBoxService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[GoneBox] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

func (s *GoneBoxService) CreateWithError() (string, error) {
	body, status, err := independentTempJSONRequest(s.client, http.MethodPost, s.apiBaseURL, "/inboxes", nil, map[string]string{"domain": s.domain})
	if err != nil {
		return "", err
	}
	if status/100 != 2 {
		return "", fmt.Errorf("GoneBox 创建邮箱 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	var data struct {
		Success bool `json:"success"`
		Data    struct {
			Address string `json:"address"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return "", fmt.Errorf("GoneBox 创建邮箱 JSON 解析失败: %w body=%s", err, shortMailGWBody(body, 200))
	}
	if !data.Success || !strings.Contains(data.Data.Address, "@") {
		return "", fmt.Errorf("GoneBox 未返回邮箱地址: %s", shortMailGWBody(body, 200))
	}
	s.address = strings.ToLower(strings.TrimSpace(data.Data.Address))
	log.Printf("[GoneBox] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

func (s *GoneBoxService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	return waitIndependentTempCode("GoneBox", s.address, timeoutSec, intervalSec, s.mailTextAndIDs, s.messageText, s.checkedIDs)
}

func (s *GoneBoxService) GetAddress() string { return s.address }

func (s *GoneBoxService) mailTextAndIDs() (string, []string, error) {
	body, status, err := independentTempRequest(s.client, http.MethodGet, s.apiBaseURL, "/inboxes/"+url.PathEscape(s.address)+"/messages", nil, nil)
	if err != nil {
		return "", nil, err
	}
	if status != http.StatusOK {
		return "", nil, fmt.Errorf("GoneBox 列表 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	var data struct {
		Data struct {
			Messages []map[string]interface{} `json:"messages"`
		} `json:"data"`
		Messages []map[string]interface{} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return body, nil, nil
	}
	messages := data.Data.Messages
	if len(messages) == 0 {
		messages = data.Messages
	}
	return independentTempJSONText(body), independentTempMessageIDs(messages), nil
}

func (s *GoneBoxService) messageText(id string) (string, error) {
	body, status, err := independentTempRequest(s.client, http.MethodGet, s.apiBaseURL, "/messages/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("GoneBox 详情 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	return independentTempJSONText(body), nil
}

type OpenInboxService struct {
	client     *http.Client
	apiBaseURL string
	inboxID    string
	address    string
	checkedIDs map[string]struct{}
}

func NewOpenInboxService(proxyURL string) *OpenInboxService {
	return &OpenInboxService{
		client:     newIndependentTempHTTPClient(proxyURL),
		apiBaseURL: openInboxAPIBaseURL,
		checkedIDs: make(map[string]struct{}),
	}
}

func (s *OpenInboxService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[OpenInbox] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

func (s *OpenInboxService) CreateWithError() (string, error) {
	body, status, err := independentTempJSONRequest(s.client, http.MethodPost, s.apiBaseURL, "/inbox", nil, map[string]string{"fingerprint": openInboxFingerprint()})
	if err != nil {
		return "", err
	}
	if status/100 != 2 {
		return "", fmt.Errorf("OpenInbox 创建邮箱 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	var data struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return "", fmt.Errorf("OpenInbox 创建邮箱 JSON 解析失败: %w body=%s", err, shortMailGWBody(body, 200))
	}
	if strings.TrimSpace(data.ID) == "" || !strings.Contains(data.Email, "@") {
		return "", fmt.Errorf("OpenInbox 未返回邮箱地址: %s", shortMailGWBody(body, 200))
	}
	s.inboxID = strings.TrimSpace(data.ID)
	s.address = strings.ToLower(strings.TrimSpace(data.Email))
	log.Printf("[OpenInbox] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

func (s *OpenInboxService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	return waitIndependentTempCode("OpenInbox", s.address, timeoutSec, intervalSec, s.mailTextAndIDs, s.messageText, s.checkedIDs)
}

func (s *OpenInboxService) GetAddress() string { return s.address }

func (s *OpenInboxService) mailTextAndIDs() (string, []string, error) {
	if strings.TrimSpace(s.inboxID) == "" {
		return "", nil, fmt.Errorf("OpenInbox inbox id 为空")
	}
	body, status, err := independentTempRequest(s.client, http.MethodGet, s.apiBaseURL, "/emails/inbox/"+url.PathEscape(s.inboxID), map[string]string{"page": "1", "limit": "30"}, nil)
	if err != nil {
		return "", nil, err
	}
	if status != http.StatusOK {
		return "", nil, fmt.Errorf("OpenInbox 列表 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	var data struct {
		Emails []map[string]interface{} `json:"emails"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return body, nil, nil
	}
	return independentTempJSONText(body), independentTempMessageIDs(data.Emails), nil
}

func (s *OpenInboxService) messageText(id string) (string, error) {
	body, status, err := independentTempRequest(s.client, http.MethodGet, s.apiBaseURL, "/emails/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("OpenInbox 详情 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	return independentTempJSONText(body), nil
}

type BlinkBoxService struct {
	client   *http.Client
	baseURL  string
	csrf     string
	snapshot string
	address  string
}

type blinkBoxLivewireResponse struct {
	Components []struct {
		Snapshot string `json:"snapshot"`
		Effects  struct {
			HTML       string `json:"html"`
			Dispatches []struct {
				Name   string                 `json:"name"`
				Params map[string]interface{} `json:"params"`
			} `json:"dispatches"`
		} `json:"effects"`
	} `json:"components"`
}

func NewBlinkBoxService(proxyURL string) *BlinkBoxService {
	return &BlinkBoxService{
		client:  newIndependentTempHTTPClient(proxyURL),
		baseURL: blinkBoxBaseURL,
	}
}

func (s *BlinkBoxService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[BlinkBox] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

func (s *BlinkBoxService) CreateWithError() (string, error) {
	body, status, err := independentTempRequest(s.client, http.MethodGet, s.baseURL, "/", nil, nil)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("BlinkBox 首页 HTTP %d: %s", status, shortMailGWBody(body, 200))
	}
	s.csrf = firstRegexpGroup(blinkBoxCSRFRe, body)
	s.snapshot = stdhtml.UnescapeString(firstRegexpGroup(blinkBoxSnapshotRe, body))
	if s.csrf == "" || s.snapshot == "" {
		return "", fmt.Errorf("BlinkBox 首页缺少 Livewire token/snapshot")
	}
	resp, raw, err := s.livewireCall("generateRandomEmail")
	if err != nil {
		return "", err
	}
	address := blinkBoxGeneratedAddress(resp)
	if address == "" {
		address = firstEmailLike(raw)
	}
	if !strings.Contains(address, "@") {
		return "", fmt.Errorf("BlinkBox 未返回邮箱地址: %s", shortMailGWBody(raw, 200))
	}
	s.address = strings.ToLower(strings.TrimSpace(address))
	if _, _, err := s.livewireCall("setActiveEmail", s.address); err != nil {
		return "", err
	}
	log.Printf("[BlinkBox] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

func (s *BlinkBoxService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.address) == "" || strings.TrimSpace(s.snapshot) == "" {
		return "", fmt.Errorf("BlinkBox 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = independentTempPollInterval
	}
	log.Printf("[BlinkBox] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		resp, raw, err := s.livewireCall("loadEmails")
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[BlinkBox] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		for _, component := range resp.Components {
			if code := blinkBoxCodeFromHTML(component.Effects.HTML); code != "" {
				log.Printf("[BlinkBox] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		if attempt%5 == 0 && strings.TrimSpace(raw) != "" {
			log.Printf("[BlinkBox] 暂无新邮件")
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

func (s *BlinkBoxService) GetAddress() string { return s.address }

func (s *BlinkBoxService) livewireCall(method string, params ...interface{}) (blinkBoxLivewireResponse, string, error) {
	var out blinkBoxLivewireResponse
	if strings.TrimSpace(s.csrf) == "" || strings.TrimSpace(s.snapshot) == "" {
		return out, "", fmt.Errorf("BlinkBox Livewire 会话未初始化")
	}
	if params == nil {
		params = []interface{}{}
	}
	payload := map[string]interface{}{
		"_token": s.csrf,
		"components": []map[string]interface{}{{
			"snapshot": s.snapshot,
			"updates":  map[string]interface{}{},
			"calls": []map[string]interface{}{{
				"path":   "",
				"method": method,
				"params": params,
			}},
		}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return out, "", err
	}
	rawURL := strings.TrimRight(s.baseURL, "/") + "/livewire/update"
	req, err := http.NewRequest(http.MethodPost, rawURL, strings.NewReader(string(data)))
	if err != nil {
		return out, "", err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", strings.TrimRight(s.baseURL, "/"))
	req.Header.Set("Referer", strings.TrimRight(s.baseURL, "/")+"/")
	req.Header.Set("X-Livewire", "true")
	resp, err := s.client.Do(req)
	if err != nil {
		return out, "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return out, "", err
	}
	body := string(respBody)
	if resp.StatusCode != http.StatusOK {
		return out, body, fmt.Errorf("BlinkBox Livewire %s HTTP %d: %s", method, resp.StatusCode, shortMailGWBody(body, 200))
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return out, body, fmt.Errorf("BlinkBox Livewire %s JSON 解析失败: %w body=%s", method, err, shortMailGWBody(body, 200))
	}
	for _, component := range out.Components {
		if strings.TrimSpace(component.Snapshot) != "" {
			s.snapshot = component.Snapshot
			break
		}
	}
	return out, body, nil
}

func blinkBoxGeneratedAddress(resp blinkBoxLivewireResponse) string {
	for _, component := range resp.Components {
		for _, dispatch := range component.Effects.Dispatches {
			if dispatch.Name != "emailGenerated" {
				continue
			}
			if address, ok := dispatch.Params["address"].(string); ok && strings.Contains(address, "@") {
				return strings.ToLower(strings.TrimSpace(address))
			}
		}
	}
	return ""
}

func firstRegexpGroup(re *regexp.Regexp, text string) string {
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func firstEmailLike(text string) string {
	re := regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	return strings.ToLower(strings.TrimSpace(re.FindString(text)))
}

func blinkBoxCodeFromHTML(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	lower := strings.ToLower(raw)
	for _, marker := range []string{"verification code", "验证码"} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		end := idx + 2000
		if end > len(raw) {
			end = len(raw)
		}
		segment := raw[idx:end]
		text := stdhtml.UnescapeString(blinkBoxTagRe.ReplaceAllString(segment, " "))
		if code := codeFromProviderText(text); code != "" {
			return code
		}
	}
	text := stdhtml.UnescapeString(blinkBoxTagRe.ReplaceAllString(raw, " "))
	return codeFromProviderText(text)
}

func waitIndependentTempCode(label, address string, timeoutSec, intervalSec int, list func() (string, []string, error), detail func(string) (string, error), checked map[string]struct{}) (string, error) {
	if strings.TrimSpace(address) == "" {
		return "", fmt.Errorf("%s 邮箱未创建", label)
	}
	if intervalSec <= 0 {
		intervalSec = independentTempPollInterval
	}
	log.Printf("[%s] 开始等待验证码: %s", label, address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		text, ids, err := list()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[%s] 获取邮件列表失败: %v", label, err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if code := codeFromProviderText(text); code != "" {
			log.Printf("[%s] 成功提取验证码: %s", label, code)
			return code, nil
		}
		for _, id := range ids {
			if _, ok := checked[id]; ok {
				continue
			}
			checked[id] = struct{}{}
			text, err := detail(id)
			if err != nil {
				log.Printf("[%s] 获取邮件详情失败: %v", label, err)
				continue
			}
			if code := codeFromProviderText(text); code != "" {
				log.Printf("[%s] 成功提取验证码: %s", label, code)
				return code, nil
			}
		}
		if attempt%5 == 0 {
			log.Printf("[%s] 暂无新邮件", label)
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

func independentTempJSONText(body string) string {
	var data interface{}
	var b strings.Builder
	if err := json.Unmarshal([]byte(body), &data); err == nil {
		appendValueText(&b, data)
		return b.String()
	}
	return body
}

func independentTempMessageIDs(messages []map[string]interface{}) []string {
	ids := []string{}
	for i, msg := range messages {
		id := ""
		for _, key := range []string{"id", "_id", "message_id", "messageId", "mail_id", "mailId", "hash_id", "hashId"} {
			id = strings.TrimSpace(fmt.Sprint(msg[key]))
			if id != "" && id != "<nil>" {
				break
			}
		}
		if id == "" {
			id = fmt.Sprintf("inline-%d", i)
		}
		ids = append(ids, id)
	}
	return ids
}

func openInboxFingerprint() string {
	sum := sha256.Sum256([]byte(randomTempMailboxLocal() + time.Now().UTC().Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", sum[:])
}

func newIndependentTempHTTPClient(proxyURL string) *http.Client {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	client := httpClientWithProxy(runtimeProxyURL, emailRequestTimeout)
	jar, _ := cookiejar.New(nil)
	client.Jar = jar
	return client
}

func independentTempJSONRequest(client *http.Client, method, baseURL, path string, params map[string]string, payload interface{}) (string, int, error) {
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
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return "", 0, err
		}
		body = strings.NewReader(string(data))
	}
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Referer", strings.TrimRight(baseURL, "/")+"/")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
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
