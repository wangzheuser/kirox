package email

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/base64"
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

const dropMailPollInterval = 3

var (
	dropMailAPIBaseURL = "https://dropmail.me/api/graphql"
	dropMailAppKey     = "==gNyAjMfRXZyNWZz9FbxhGchJ3Zf1Gd"
	dropMailCodeRegex  = regexp.MustCompile(`(^|[^0-9])([0-9]{6})([^0-9]|$)`)
	dropMailDomains    = []string{
		"mailtowin.com",
		"mail2me.co",
		"pickmemail.com",
		"maximail.vip",
		"emlpro.com",
		"freeml.net",
	}
)

// DropMailService 提供 DropMail 零配置临时邮箱能力。
type DropMailService struct {
	client         *http.Client
	baseURL        string
	token          string
	sessionID      string
	address        string
	checkedIDs     map[string]struct{}
	tokenGenerator func() (string, error)
}

type dropMailGraphQLResponse struct {
	Data   map[string]json.RawMessage `json:"data"`
	Errors []map[string]interface{}   `json:"errors"`
}

type dropMailDomain struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type dropMailAddress struct {
	Address string `json:"address"`
}

type dropMailSession struct {
	ID        string            `json:"id"`
	Addresses []dropMailAddress `json:"addresses"`
	Mails     []dropMailMessage `json:"mails"`
}

type dropMailMessage struct {
	ID            string `json:"id"`
	FromAddr      string `json:"fromAddr"`
	HeaderFrom    string `json:"headerFrom"`
	HeaderSubject string `json:"headerSubject"`
	Text          string `json:"text"`
	HTML          string `json:"html"`
	Raw           string `json:"raw"`
}

// NewDropMailService 创建 DropMail 临时邮箱服务。
func NewDropMailService(proxyURL string) *DropMailService {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	return &DropMailService{
		client:         httpClientWithProxy(runtimeProxyURL, emailRequestTimeout),
		baseURL:        dropMailAPIBaseURL,
		checkedIDs:     make(map[string]struct{}),
		tokenGenerator: generateDropMailToken,
	}
}

// Create 创建临时邮箱，兼容 TempEmailService 接口。
func (s *DropMailService) Create() string {
	address, err := s.CreateWithError()
	if err != nil {
		log.Printf("[DropMail] 创建邮箱失败: %v", err)
		return ""
	}
	return address
}

// CreateWithError 创建 DropMail 匿名会话，并优先选择实测可通过 AWS/TES 的 mailtowin.com 域名。
func (s *DropMailService) CreateWithError() (string, error) {
	token, err := s.tokenGenerator()
	if err != nil {
		return "", err
	}
	s.token = token
	domains, err := s.getDomains()
	if err != nil {
		return "", err
	}
	domainID := preferredDropMailDomainID(domains)
	input := map[string]interface{}{"withAddress": true}
	if domainID != "" {
		input["domainId"] = domainID
	}
	session, err := s.introduceSession(input)
	if err != nil {
		return "", err
	}
	if len(session.Addresses) == 0 || strings.TrimSpace(session.Addresses[0].Address) == "" {
		return "", fmt.Errorf("DropMail 未返回邮箱地址")
	}
	s.sessionID = session.ID
	s.address = strings.ToLower(strings.TrimSpace(session.Addresses[0].Address))
	log.Printf("[DropMail] 邮箱生成成功: %s", s.address)
	return s.address, nil
}

// WaitForCode 轮询等待 AWS/Kiro 注册验证码。
func (s *DropMailService) WaitForCode(timeoutSec, intervalSec int) (string, error) {
	if strings.TrimSpace(s.token) == "" || strings.TrimSpace(s.sessionID) == "" || strings.TrimSpace(s.address) == "" {
		return "", fmt.Errorf("DropMail 邮箱未创建")
	}
	if intervalSec <= 0 {
		intervalSec = dropMailPollInterval
	}
	log.Printf("[DropMail] 开始等待验证码: %s", s.address)
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	attempt := 0
	for time.Now().Before(deadline) {
		attempt++
		messages, err := s.listMessages()
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[DropMail] 获取邮件列表失败: %v", err)
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		if len(messages) == 0 {
			if attempt%5 == 0 {
				log.Printf("[DropMail] 暂无新邮件")
			}
			time.Sleep(time.Duration(intervalSec) * time.Second)
			continue
		}
		for _, msg := range messages {
			id := strings.TrimSpace(msg.ID)
			if id == "" {
				id = fmt.Sprintf("%s|%s|%s", msg.FromAddr, msg.HeaderFrom, msg.HeaderSubject)
			}
			if _, ok := s.checkedIDs[id]; ok {
				continue
			}
			s.checkedIDs[id] = struct{}{}
			sender := firstDropMailText(msg.HeaderFrom, msg.FromAddr)
			subject := msg.HeaderSubject
			log.Printf("[DropMail] 发现邮件 - 发件人: %s, 主题: %s", sender, subject)
			combined := strings.Join([]string{subject, sender, msg.Text, mailGWHTMLToText(msg.HTML), msg.Raw}, "\n")
			if !mailGWLooksLikeAWSVerification(subject, sender, combined) {
				continue
			}
			if code := dropMailCodeFromText(combined); code != "" {
				log.Printf("[DropMail] 成功提取验证码: %s", code)
				return code, nil
			}
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeoutSec)
}

// GetAddress 获取当前邮箱地址。
func (s *DropMailService) GetAddress() string { return s.address }

func (s *DropMailService) getDomains() ([]dropMailDomain, error) {
	body, err := s.graphQL(`query Domains { domains(includeBroken:false) { id name } }`, nil)
	if err != nil {
		return nil, err
	}
	payload, err := parseDropMailGraphQL(body)
	if err != nil {
		return nil, err
	}
	var domains []dropMailDomain
	if err := json.Unmarshal(payload["domains"], &domains); err != nil {
		return nil, err
	}
	return domains, nil
}

func (s *DropMailService) introduceSession(input map[string]interface{}) (dropMailSession, error) {
	body, err := s.graphQL(`mutation IntroduceSession($input: IntroduceSessionInput) { introduceSession(input: $input) { id expiresAt addresses { id address restoreKey } } }`, map[string]interface{}{"input": input})
	if err != nil {
		return dropMailSession{}, err
	}
	payload, err := parseDropMailGraphQL(body)
	if err != nil {
		return dropMailSession{}, err
	}
	var session dropMailSession
	if err := json.Unmarshal(payload["introduceSession"], &session); err != nil {
		return dropMailSession{}, err
	}
	return session, nil
}

func (s *DropMailService) listMessages() ([]dropMailMessage, error) {
	body, err := s.graphQL(`query Session($id: ID!) { session(id: $id) { mails { id fromAddr headerFrom headerSubject text html raw receivedAt } } }`, map[string]interface{}{"id": s.sessionID})
	if err != nil {
		return nil, err
	}
	payload, err := parseDropMailGraphQL(body)
	if err != nil {
		return nil, err
	}
	var session dropMailSession
	if err := json.Unmarshal(payload["session"], &session); err != nil {
		return nil, err
	}
	return session.Mails, nil
}

func (s *DropMailService) graphQL(query string, variables interface{}) ([]byte, error) {
	if strings.TrimSpace(s.token) == "" {
		return nil, fmt.Errorf("DropMail token 为空")
	}
	payload := map[string]interface{}{"query": query}
	if variables != nil {
		payload["variables"] = variables
	}
	raw, _ := json.Marshal(payload)
	endpoint := strings.TrimRight(s.baseURL, "/") + "/" + url.PathEscape(s.token)
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mailTempUserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", "https://dropmail.me/")
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
		return nil, fmt.Errorf("DropMail GraphQL HTTP %d: %s", resp.StatusCode, shortMailGWBody(string(body), 300))
	}
	return body, nil
}

func parseDropMailGraphQL(body []byte) (map[string]json.RawMessage, error) {
	var payload dropMailGraphQLResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 DropMail GraphQL 失败: %w", err)
	}
	if len(payload.Errors) > 0 {
		return nil, fmt.Errorf("DropMail GraphQL errors: %v", payload.Errors)
	}
	return payload.Data, nil
}

func preferredDropMailDomainID(domains []dropMailDomain) string {
	byName := make(map[string]string, len(domains))
	for _, domain := range domains {
		name := strings.ToLower(strings.TrimSpace(domain.Name))
		if name != "" && strings.TrimSpace(domain.ID) != "" {
			byName[name] = domain.ID
		}
	}
	for _, name := range dropMailDomains {
		if id := byName[strings.ToLower(name)]; id != "" {
			return id
		}
	}
	if len(domains) > 0 {
		return strings.TrimSpace(domains[0].ID)
	}
	return ""
}

func generateDropMailToken() (string, error) {
	key, err := decodeDropMailAppKey(dropMailAppKey)
	if err != nil {
		return "", err
	}
	now := time.Now()
	randomPart := fmt.Sprintf("%04d%02d%02d%s", now.Year(), int(now.Month()), now.Day(), randomDropMailString(16))
	return fmt.Sprintf("website_%s_%s", randomPart, dropMailHash(randomPart+key)), nil
}

func decodeDropMailAppKey(encoded string) (string, error) {
	runes := []rune(strings.TrimSpace(encoded))
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	data, err := base64.StdEncoding.DecodeString(string(runes))
	if err != nil {
		return "", fmt.Errorf("解析 DropMail app key 失败: %w", err)
	}
	return string(data), nil
}

func randomDropMailString(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	raw := make([]byte, n)
	if _, err := cryptorand.Read(raw); err == nil {
		var b strings.Builder
		for _, v := range raw {
			b.WriteByte(alphabet[int(v)%len(alphabet)])
		}
		return b.String()
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(alphabet[time.Now().UnixNano()%int64(len(alphabet))])
	}
	return b.String()
}

func dropMailHash(text string) string {
	var hash uint32 = 2166136261
	for _, r := range text {
		hash ^= uint32(r)
		hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24)
	}
	return fmt.Sprintf("%x", hash)
}

func dropMailCodeFromText(text string) string {
	cleaned := mailGWHTMLToText(text)
	matches := dropMailCodeRegex.FindAllStringSubmatch(cleaned, -1)
	for _, match := range matches {
		if len(match) > 2 && match[2] != "000000" {
			return match[2]
		}
	}
	return ""
}

func firstDropMailText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
