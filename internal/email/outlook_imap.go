package email

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"reg_go/internal/proxy"
	"reg_go/internal/storage"
)

// OutlookAccount Outlook 邮箱账号
type OutlookAccount struct {
	Email        string
	Password     string
	ClientID     string
	RefreshToken string
}

// recipientMatches 判断邮件收件人是否命中当前注册的别名邮箱。
// 别名邮箱（如 a001@outlook.jp 与 a002@outlook.jp）共享同一物理收件箱，
// 并发注册时收件箱会同时收到发往不同别名的验证码邮件，必须按收件人区分，
// 否则会把别的别名的验证码误当成自己的。
// toField 可能是 "Name <a001@outlook.jp>"、"a001@outlook.jp" 或逗号分隔的多个地址，
// 故采用规范化(小写+去空格)后的包含匹配而非全等。
// 注意：当 target 为空（理论上不应发生）时返回 true 以保留旧行为，避免误伤。
func recipientMatches(toField, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return true
	}
	return strings.Contains(strings.ToLower(toField), target)
}

// ParseOutlookCSV 解析 outlook.csv
func ParseOutlookCSV(path string) ([]OutlookAccount, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var accounts []OutlookAccount
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "----", 4)
		if len(parts) != 4 {
			log.Printf("跳过格式错误的行: %s", line[:min(50, len(line))])
			continue
		}
		accounts = append(accounts, OutlookAccount{
			Email:        parts[0],
			Password:     parts[1],
			ClientID:     parts[2],
			RefreshToken: parts[3],
		})
	}
	return accounts, nil
}

// ParseOutlookLines 从文本内容直接解析 Outlook 账号 (Web UI 使用)
// 支持两种格式:
// 1. 换行分隔: 每行一个账号
// 2. 空格分隔: 账号之间用空格隔开
func ParseOutlookLines(data string) []OutlookAccount {
	var accounts []OutlookAccount
	data = strings.TrimSpace(data)
	if data == "" {
		return accounts
	}

	// 先尝试按换行分割
	lines := strings.Split(data, "\n")

	// 如果只有一行，可能是空格分隔的格式
	if len(lines) == 1 {
		// 尝试按空格分割（账号格式: email----password----clientid----token）
		// 每个账号以空格结尾，下一个账号开始
		parts := strings.Fields(data) // Fields 会按空白字符分割并去除空白
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			fields := strings.SplitN(part, "----", 4)
			if len(fields) == 4 {
				accounts = append(accounts, OutlookAccount{
					Email:        strings.TrimSpace(fields[0]),
					Password:     strings.TrimSpace(fields[1]),
					ClientID:     strings.TrimSpace(fields[2]),
					RefreshToken: strings.TrimSpace(fields[3]),
				})
			}
		}
	} else {
		// 多行格式，按行解析
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "----", 4)
			if len(parts) == 4 {
				accounts = append(accounts, OutlookAccount{
					Email:        strings.TrimSpace(parts[0]),
					Password:     strings.TrimSpace(parts[1]),
					ClientID:     strings.TrimSpace(parts[2]),
					RefreshToken: strings.TrimSpace(parts[3]),
				})
			}
		}
	}

	return accounts
}

// RefreshOutlookToken 用 refresh_token 获取 access_token（优先走全局代理，失败时降级直连）
func RefreshOutlookToken(acc OutlookAccount) (string, error) {
	return refreshOutlookToken(acc, storage.GetEmailProxy(), true)
}

// RefreshOutlookTokenWithProxy 用指定代理刷新 Outlook access_token。
func RefreshOutlookTokenWithProxy(acc OutlookAccount, proxyURL string) (string, error) {
	return refreshOutlookToken(acc, proxyURL, false)
}

// refreshOutlookToken 用 refresh_token 获取 access_token，可按调用场景控制是否允许降级直连。
func refreshOutlookToken(acc OutlookAccount, proxyURL string, allowDirectFallback bool) (string, error) {
	form := url.Values{
		"client_id":     {acc.ClientID},
		"refresh_token": {acc.RefreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {"https://outlook.office.com/IMAP.AccessAsUser.All offline_access"},
	}

	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	tryPost := func(p string) (resp *http.Response, err error) {
		client := httpClientWithProxy(p, emailRequestTimeout)
		return client.Post(
			"https://login.microsoftonline.com/consumers/oauth2/v2.0/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
	}
	resp, err := tryPost(runtimeProxyURL)
	if err != nil && runtimeProxyURL != "" && allowDirectFallback {
		log.Printf("[Outlook OAuth] 代理请求失败，降级直连：%v", err)
		resp, err = tryPost("")
	}
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("刷新失败 %d: %s", resp.StatusCode, string(body[:min(300, len(body))]))
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)
	token, _ := result["access_token"].(string)
	if token == "" {
		return "", fmt.Errorf("响应中无 access_token")
	}
	return token, nil
}

// buildXOAuth2 构建 XOAUTH2 认证字符串
func buildXOAuth2(email, accessToken string) string {
	auth := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", email, accessToken)
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// imapClient 简易 IMAP 客户端
type imapClient struct {
	conn   net.Conn
	reader *bufio.Reader
	tag    int
}

// newIMAPClient 连接 Outlook IMAP（优先走全局代理，代理被封端口时自动降级直连）
func newIMAPClient() (*imapClient, error) {
	return newIMAPClientWithFallback(storage.GetEmailProxy(), true)
}

// newIMAPClientWithProxy 连接 Outlook IMAP，并优先使用指定代理。
func newIMAPClientWithProxy(proxyURL string) (*imapClient, error) {
	return newIMAPClientWithFallback(proxyURL, false)
}

// newIMAPClientWithFallback 连接 Outlook IMAP，可按调用场景控制是否允许降级直连。
func newIMAPClientWithFallback(proxyURL string, allowDirectFallback bool) (*imapClient, error) {
	const target = "outlook.office365.com:993"
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	rawConn, err := dialThroughProxy(runtimeProxyURL, "tcp", target, emailRequestTimeout)
	if err != nil && runtimeProxyURL != "" && allowDirectFallback {
		log.Printf("[IMAP] 代理拨号失败，降级直连：%v", err)
		rawConn, err = dialThroughProxy("", "tcp", target, emailRequestTimeout)
	}
	if err != nil {
		return nil, fmt.Errorf("连接失败: %v", err)
	}
	tlsConfig := &tls.Config{ServerName: "outlook.office365.com"}
	conn := tls.Client(rawConn, tlsConfig)
	if err := conn.SetDeadline(time.Now().Add(emailRequestTimeout)); err == nil {
		err = conn.Handshake()
		conn.SetDeadline(time.Time{})
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("TLS 握手失败: %v", err)
		}
	}

	c := &imapClient{conn: conn, reader: bufio.NewReader(conn), tag: 0}
	greeting, err := c.readLine()
	if err != nil {
		conn.Close()
		return nil, err
	}
	log.Printf("[IMAP] %s", greeting)
	return c, nil
}

func (c *imapClient) sendCommand(cmd string) (string, error) {
	c.tag++
	tagStr := fmt.Sprintf("A%03d", c.tag)
	line := fmt.Sprintf("%s %s\r\n", tagStr, cmd)
	_ = c.conn.SetDeadline(time.Now().Add(emailRequestTimeout))
	defer c.conn.SetDeadline(time.Time{})
	_, err := c.conn.Write([]byte(line))
	if err != nil {
		return "", err
	}
	return tagStr, nil
}

func (c *imapClient) readLine() (string, error) {
	_ = c.conn.SetDeadline(time.Now().Add(emailRequestTimeout))
	defer c.conn.SetDeadline(time.Time{})
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (c *imapClient) readUntilTag(tag string) ([]string, string, error) {
	var lines []string
	for {
		line, err := c.readLine()
		if err != nil {
			return lines, "", err
		}
		if strings.HasPrefix(line, tag+" ") {
			return lines, line, nil
		}
		lines = append(lines, line)
	}
}

func (c *imapClient) authenticate(email, accessToken string) error {
	xoauth2 := buildXOAuth2(email, accessToken)
	tag, err := c.sendCommand("AUTHENTICATE XOAUTH2 " + xoauth2)
	if err != nil {
		return err
	}
	_, result, err := c.readUntilTag(tag)
	if err != nil {
		return err
	}
	if !strings.Contains(result, "OK") {
		return fmt.Errorf("认证失败: %s", result)
	}
	log.Println("[IMAP] 认证成功")

	// 发送 NOOP 确保会话完全就绪（Outlook 有时认证后需要额外握手）
	for i := 0; i < 3; i++ {
		noopTag, err := c.sendCommand("NOOP")
		if err != nil {
			return err
		}
		_, noopResult, err := c.readUntilTag(noopTag)
		if err != nil {
			return err
		}
		if strings.Contains(noopResult, "OK") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Outlook Exchange 后端认证后需要额外时间建立 mailbox 连接，否则 SELECT 会返回 "not connected"
	time.Sleep(2 * time.Second)
	return nil
}

func (c *imapClient) selectInbox() (int, error) {
	tag, err := c.sendCommand("SELECT INBOX")
	if err != nil {
		return 0, err
	}
	lines, result, err := c.readUntilTag(tag)
	if err != nil {
		return 0, err
	}
	if strings.Contains(result, "OK") {
		total := 0
		for _, line := range lines {
			if strings.Contains(line, "EXISTS") {
				fmt.Sscanf(line, "* %d EXISTS", &total)
			}
		}
		return total, nil
	}
	// "not connected" 表示 Outlook 后端尚未就绪，同连接重试无效，由调用方重连后重试
	errMsg := strings.TrimSpace(result)
	if len(errMsg) > 80 {
		errMsg = errMsg[:80] + "..."
	}
	return 0, fmt.Errorf("SELECT 失败: %s", errMsg)
}

func (c *imapClient) close() {
	c.sendCommand("LOGOUT")
	c.conn.Close()
}

// fetchHeader 获取指定邮件的某个 header 字段值
func (c *imapClient) fetchHeader(seq int, field string) (string, error) {
	if seq <= 0 {
		return "", fmt.Errorf("无效的邮件序号")
	}
	tag, err := c.sendCommand(fmt.Sprintf("FETCH %d (BODY.PEEK[HEADER.FIELDS (%s)])", seq, field))
	if err != nil {
		return "", err
	}
	lines, result, err := c.readUntilTag(tag)
	if err != nil {
		return "", err
	}
	if !strings.Contains(result, "OK") {
		return "", fmt.Errorf("FETCH HEADER 失败: %s", result)
	}
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, strings.ToLower(field)+":") {
			return strings.TrimSpace(line[len(field)+1:]), nil
		}
	}
	return "", nil
}

// fetchLatestBody 获取指定邮件的正文并解码
func (c *imapClient) fetchLatestBody(seq int) (string, error) {
	if seq <= 0 {
		return "", fmt.Errorf("无效的邮件序号")
	}
	tag, err := c.sendCommand(fmt.Sprintf("FETCH %d (BODY.PEEK[TEXT])", seq))
	if err != nil {
		return "", err
	}
	lines, result, err := c.readUntilTag(tag)
	if err != nil {
		return "", err
	}
	if !strings.Contains(result, "OK") {
		return "", fmt.Errorf("FETCH TEXT 失败: %s", result)
	}

	var rawLines []string
	inBody := false
	for _, line := range lines {
		if strings.Contains(line, "FETCH") {
			inBody = true
			continue
		}
		if line == ")" {
			continue
		}
		if inBody {
			rawLines = append(rawLines, line)
		}
	}

	raw := strings.Join(rawLines, "\n")

	// 尝试解码 MIME base64 内容
	parts := strings.Split(raw, "------=_Part_")
	var decoded string
	for _, part := range parts {
		if strings.Contains(part, "base64") {
			idx := strings.Index(part, "base64")
			content := part[idx+6:]
			b64 := strings.Map(func(r rune) rune {
				if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
					return -1
				}
				return r
			}, content)
			if data, err := base64.StdEncoding.DecodeString(b64); err == nil {
				decoded += string(data) + " "
			}
		}
	}
	if decoded != "" {
		return decoded, nil
	}

	// 整体 base64 解码
	cleaned := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, raw)
	if data, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
		return string(data), nil
	}

	return raw, nil
}

// WaitForOTP 通过 IMAP 轮询等待 AWS 验证码
func WaitForOTP(ctx context.Context, acc OutlookAccount, beforeCount, timeout, interval int) (string, error) {
	return WaitForOTPWithProxy(ctx, acc, beforeCount, timeout, interval, storage.GetEmailProxy())
}

// WaitForOTPWithProxy 通过指定代理轮询等待 AWS 验证码。
// 支持 context 取消，任务停止时立即中断轮询。
func WaitForOTPWithProxy(ctx context.Context, acc OutlookAccount, beforeCount, timeout, interval int, proxyURL string) (string, error) {
	log.Printf("[Outlook IMAP] 等待验证码, 邮箱=%s, 发送前邮件数=%d", acc.Email, beforeCount)

	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	accessToken, err := RefreshOutlookTokenWithProxy(acc, runtimeProxyURL)
	if err != nil {
		return "", fmt.Errorf("刷新 Outlook Token 失败: %v", err)
	}

	codeRegex := regexp.MustCompile(`\b(\d{6})\b`)
	maxRetries := timeout / interval
	consecutiveSelectFail := 0
	maxConsecutiveSelectFail := 3 // 连续 3 次 SELECT 失败则提前放弃，避免单账号卡住整批
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 每次轮询前检查 context 是否已取消
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		client, err := newIMAPClientWithProxy(runtimeProxyURL)
		if err != nil {
			if attempt%5 == 0 {
				log.Printf("[Outlook IMAP] 连接失败: %v, 重试中...", err)
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(interval) * time.Second):
			}
			continue
		}

		if err := client.authenticate(acc.Email, accessToken); err != nil {
			client.close()
			accessToken, _ = RefreshOutlookTokenWithProxy(acc, runtimeProxyURL)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(interval) * time.Second):
			}
			continue
		}

		total, err := client.selectInbox()
		if err != nil {
			client.close()
			consecutiveSelectFail++
			if consecutiveSelectFail >= maxConsecutiveSelectFail {
				log.Printf("[Outlook IMAP] 邮箱 %s 连续 %d 次 SELECT 失败，放弃等待", acc.Email, consecutiveSelectFail)
				return "", fmt.Errorf("IMAP SELECT 连续失败 %d 次: %v", consecutiveSelectFail, err)
			}
			log.Printf("[Outlook IMAP] SELECT 失败 (%d/%d): %v", consecutiveSelectFail, maxConsecutiveSelectFail, err)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(interval) * time.Second):
			}
			continue
		}
		consecutiveSelectFail = 0 // 成功则重置

		if total <= beforeCount {
			client.close()
			if attempt%5 == 0 {
				log.Printf("[Outlook IMAP] [%d/%d] 暂无新邮件 (当前%d封)...", attempt, maxRetries, total)
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(interval) * time.Second):
			}
			continue
		}

		for i := total; i > beforeCount; i-- {
			// 获取 To header，校验邮件收件人是否为当前注册的别名邮箱。
			// 共享收件箱下并发注册时，需按收件人过滤，避免拿到其他别名的验证码。
			toHeader, _ := client.fetchHeader(i, "TO")
			toHeader = strings.TrimSpace(toHeader)
			if toHeader == "" {
				// To 缺失时无法判别归属，保留旧行为继续尝试，但记录告警以便排查。
				log.Printf("[Outlook IMAP] 警告: 邮件 seq=%d 缺少 To 字段，无法校验收件人，按旧逻辑处理", i)
			} else if !recipientMatches(toHeader, acc.Email) {
				// 收件人不匹配当前别名，跳过该邮件，避免验证码错配。
				log.Printf("[Outlook IMAP] 跳过非本别名邮件: seq=%d, To=%s, 期望=%s", i, toHeader, acc.Email)
				continue
			}

			body, err := client.fetchLatestBody(i)
			if err != nil {
				continue
			}
			code := extractCodeFromText(body, codeRegex)
			if code != "" {
				log.Printf("[Outlook IMAP] 获取到验证码: %s", code)
				client.close()
				return code, nil
			}
		}

		client.close()
		if attempt%5 == 0 {
			log.Printf("[Outlook IMAP] [%d/%d] 新邮件中未找到验证码...", attempt, maxRetries)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
	return "", fmt.Errorf("等待验证码超时 (%ds)", timeout)
}

// GetInboxCount 获取收件箱当前邮件数量（带完整重连重试）
func GetInboxCount(acc OutlookAccount) (int, error) {
	return GetInboxCountWithProxy(acc, storage.GetEmailProxy())
}

// GetInboxCountWithProxy 通过指定代理获取收件箱当前邮件数量。
func GetInboxCountWithProxy(acc OutlookAccount, proxyURL string) (int, error) {
	runtimeProxyURL := proxy.RenderURLTemplate(proxyURL)
	accessToken, err := RefreshOutlookTokenWithProxy(acc, runtimeProxyURL)
	if err != nil {
		return 0, fmt.Errorf("刷新 Outlook Token 失败: %v", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1+attempt) * time.Second)
		}
		client, err := newIMAPClientWithProxy(runtimeProxyURL)
		if err != nil {
			lastErr = fmt.Errorf("连接 IMAP 失败: %v", err)
			continue
		}
		if err := client.authenticate(acc.Email, accessToken); err != nil {
			client.close()
			lastErr = fmt.Errorf("IMAP 认证失败: %v", err)
			continue
		}
		total, err := client.selectInbox()
		if err != nil {
			client.close()
			lastErr = fmt.Errorf("选择收件箱失败: %v", err)
			log.Printf("[IMAP] GetInboxCount 失败，重连重试 %d/3...", attempt+1)
			continue
		}
		client.close()
		return total, nil
	}
	return 0, lastErr
}
