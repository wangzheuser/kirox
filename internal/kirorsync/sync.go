package kirorsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SyncResult 同步结果
type SyncResult struct {
	Total   int          `json:"total"`
	Success int          `json:"success"`
	Failed  int          `json:"failed"`
	Error   string       `json:"error,omitempty"`
	Details []SyncDetail `json:"details"`
}

// SyncDetail 单条同步明细
type SyncDetail struct {
	Email             string `json:"email"`
	Success           bool   `json:"success"`
	CredentialID      int    `json:"credentialId,omitempty"`
	Error             string `json:"error,omitempty"`
	Rejected          bool   `json:"rejected,omitempty"`
	RejectReason      string `json:"rejectReason,omitempty"`
	Verified          bool   `json:"verified,omitempty"`
	VerificationError string `json:"verificationError,omitempty"`
}

// addCredentialRequest kiro.rs 添加凭据请求体
type addCredentialRequest struct {
	Email        string `json:"email"`
	RefreshToken string `json:"refreshToken"`
	AuthMethod   string `json:"authMethod,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Priority     int    `json:"priority,omitempty"`
	AuthRegion   string `json:"authRegion,omitempty"`
}

// addCredentialResponse kiro.rs 添加凭据响应体
type addCredentialResponse struct {
	Success      bool            `json:"success"`
	Message      string          `json:"message"`
	CredentialID int             `json:"credentialId"`
	Email        string          `json:"email"`
	ModelCount   int             `json:"modelCount,omitempty"`
	Balance      json.RawMessage `json:"balance,omitempty"`
}

var (
	syncMu   sync.Mutex
	syncCond = sync.NewCond(&syncMu)
	syncing  bool
	client   = &http.Client{Timeout: 10 * time.Second}
)

// SyncAccounts 将账号列表逐条推送到 kiro.rs，失败项自动重试一次。
// 并发保护：同一时刻只允许一个同步任务执行。
func SyncAccounts(apiURL, apiKey string, accounts []map[string]interface{}) SyncResult {
	return syncAccounts(apiURL, apiKey, accounts, false)
}

// SyncAccountsBlocking 将账号列表逐条推送到 kiro.rs，若已有同步正在执行则等待。
// 用于自动同步队列，避免高并发注册成功时因为同步互斥而丢弃待同步账号。
func SyncAccountsBlocking(apiURL, apiKey string, accounts []map[string]interface{}) SyncResult {
	return syncAccounts(apiURL, apiKey, accounts, true)
}

func syncAccounts(apiURL, apiKey string, accounts []map[string]interface{}, wait bool) SyncResult {
	syncMu.Lock()
	if syncing {
		if !wait {
			syncMu.Unlock()
			return SyncResult{Error: "同步正在进行中"}
		}
		for syncing {
			syncCond.Wait()
		}
	}
	syncing = true
	syncMu.Unlock()
	defer func() {
		syncMu.Lock()
		syncing = false
		syncCond.Broadcast()
		syncMu.Unlock()
	}()

	return syncAccountsLocked(apiURL, apiKey, accounts)
}

func syncAccountsLocked(apiURL, apiKey string, accounts []map[string]interface{}) SyncResult {
	// 过滤有效账号（必须有 refreshToken）
	var validAccounts []map[string]interface{}
	for _, acc := range accounts {
		rt, _ := acc["refreshToken"].(string)
		if strings.TrimSpace(rt) != "" {
			validAccounts = append(validAccounts, acc)
		}
	}

	if len(validAccounts) == 0 {
		log.Printf("[Kiro] kiro.rs 同步: 无有效账号（缺少 refreshToken），已跳过")
		return SyncResult{Total: 0, Success: 0, Failed: 0}
	}

	total := len(validAccounts)
	result := SyncResult{Total: total}

	log.Printf("[Kiro] kiro.rs 同步开始: 共 %d 个有效账号", total)

	// 逐条推送；可重试错误在当前账号上立即重试一次，避免后续账号日志插入导致难以对应。
	for i, acc := range validAccounts {
		detail := pushOne(apiURL, apiKey, acc)
		retried := false
		if !detail.Success && !detail.Rejected && isRetryableError(detail.Error) {
			log.Printf("[Kiro] kiro.rs 同步 [%d/%d] %s -> 失败(重试1/1): %s", i+1, total, detail.Email, detail.Error)
			detail = pushOne(apiURL, apiKey, acc)
			retried = true
		}

		if detail.Success {
			result.Success++
			prefix := "[Kiro] kiro.rs 同步"
			if retried {
				prefix = "[Kiro] kiro.rs 重试"
			}
			if detail.Error != "" {
				log.Printf("%s [%d/%d] %s -> 已存在，视为成功", prefix, i+1, total, detail.Email)
			} else {
				log.Printf("%s [%d/%d] %s -> 成功", prefix, i+1, total, detail.Email)
			}
		} else {
			result.Failed++
			if retried {
				log.Printf("[Kiro] kiro.rs 重试 [%d/%d] %s -> 失败: %s", i+1, total, detail.Email, detail.Error)
			} else {
				log.Printf("[Kiro] kiro.rs 同步 [%d/%d] %s -> 失败: %s", i+1, total, detail.Email, detail.Error)
			}
		}
		result.Details = append(result.Details, detail)
	}

	return result
}

// TestConnection 测试 kiro.rs 连通性和认证有效性。
func TestConnection(apiURL, apiKey string) error {
	url := strings.TrimRight(apiURL, "/") + "/api/admin/credentials"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("认证失败 (HTTP %d)，请检查 API Key", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("服务端错误 (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// pushOne 推送单个账号到 kiro.rs
func pushOne(apiURL, apiKey string, acc map[string]interface{}) SyncDetail {
	email, _ := acc["email"].(string)
	email = strings.TrimSpace(email)
	refreshToken, _ := acc["refreshToken"].(string)
	clientID, _ := acc["clientId"].(string)
	clientSecret, _ := acc["clientSecret"].(string)
	region, _ := acc["region"].(string)
	if region == "" {
		region = "us-east-1"
	}

	// priority 可能是 float64（JSON 解析）或 int
	priority := 0
	switch v := acc["priority"].(type) {
	case float64:
		priority = int(v)
	case int:
		priority = v
	}

	reqBody := addCredentialRequest{
		Email:        email,
		RefreshToken: refreshToken,
		AuthMethod:   "idc",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Priority:     priority,
		AuthRegion:   region,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	reqURL := strings.TrimRight(apiURL, "/") + "/api/admin/credentials"
	httpReq, err := http.NewRequest("POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return SyncDetail{Email: email, Success: false, Error: "构造请求失败: " + err.Error()}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return SyncDetail{Email: email, Success: false, Error: "网络错误: " + err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
		if resp.StatusCode == http.StatusBadRequest && isExistingCredentialError(string(respBody)) {
			return SyncDetail{Email: email, Success: true, Error: "凭据已存在，视为成功: " + errMsg}
		}
		if rejected, reason := isPermanentCredentialRejection(resp.StatusCode, respBody); rejected {
			return SyncDetail{Email: email, Success: false, Error: errMsg, Rejected: true, RejectReason: reason}
		}
		return SyncDetail{Email: email, Success: false, Error: errMsg}
	}

	var respData addCredentialResponse
	if err := json.Unmarshal(respBody, &respData); err != nil {
		// 状态码 2xx 但解析失败，仍视为成功；不能据此删除本地账号。
		return SyncDetail{Email: email, Success: true, Verified: false, VerificationError: "kiro.rs 响应解析失败，未完成验活"}
	}

	detail := SyncDetail{
		Email:        email,
		Success:      true,
		CredentialID: respData.CredentialID,
	}
	if hasBalance(respData.Balance) {
		detail.Verified = true
		return detail
	}
	if respData.CredentialID <= 0 {
		detail.VerificationError = "kiro.rs 未返回 balance/credentialId，未完成验活"
		return detail
	}

	verified := forceRefreshBalance(apiURL, apiKey, respData.CredentialID, email)
	if verified.Rejected {
		return verified
	}
	if verified.Verified {
		detail.Verified = true
		return detail
	}
	detail.VerificationError = verified.VerificationError
	return detail
}

func hasBalance(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func forceRefreshBalance(apiURL, apiKey string, credentialID int, email string) SyncDetail {
	reqURL := fmt.Sprintf("%s/api/admin/credentials/%d/balance?force_refresh=true", strings.TrimRight(apiURL, "/"), credentialID)
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return SyncDetail{Email: email, Success: true, CredentialID: credentialID, VerificationError: "构造验活请求失败: " + err.Error()}
	}
	req.Header.Set("x-api-key", apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return SyncDetail{Email: email, Success: true, CredentialID: credentialID, VerificationError: "网络错误: " + err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		errMsg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
		if rejected, reason := isPermanentCredentialRejection(resp.StatusCode, respBody); rejected {
			return SyncDetail{Email: email, Success: false, CredentialID: credentialID, Error: errMsg, Rejected: true, RejectReason: reason}
		}
		return SyncDetail{Email: email, Success: true, CredentialID: credentialID, VerificationError: errMsg}
	}
	return SyncDetail{Email: email, Success: true, CredentialID: credentialID, Verified: true}
}

// isRetryableError 判断错误是否可重试（网络错误或 5xx）
func isRetryableError(errMsg string) bool {
	if strings.Contains(errMsg, "网络错误") {
		return true
	}
	if strings.Contains(errMsg, "HTTP 5") {
		return true
	}
	return false
}

// isExistingCredentialError 判断 kiro.rs 返回是否表示凭据已经存在。
func isExistingCredentialError(body string) bool {
	normalized := strings.ToLower(body)
	return strings.Contains(body, "凭据已存在") ||
		strings.Contains(body, "已存在") ||
		strings.Contains(body, "refreshToken 重复") ||
		strings.Contains(normalized, "refreshtoken") && (strings.Contains(body, "重复") || strings.Contains(normalized, "duplicate") || strings.Contains(normalized, "already exists")) ||
		strings.Contains(normalized, "credential") && (strings.Contains(body, "已存在") || strings.Contains(normalized, "duplicate") || strings.Contains(normalized, "already exists"))
}

func isPermanentCredentialRejection(status int, body []byte) (bool, string) {
	text := string(body)
	message, errorType, topReason := parseKiroRSError(body)
	messageLower := strings.ToLower(message)
	errorTypeLower := strings.ToLower(errorType)
	reasonLower := strings.ToLower(topReason)
	all := strings.ToLower(strings.Join([]string{text, message, errorType, topReason}, "\n"))

	// 管理态或配置/平台错误不是本地账号永久失效依据。
	if errorTypeLower == "authentication_error" || status == http.StatusUnauthorized && strings.Contains(all, "admin") {
		return false, ""
	}
	if status == http.StatusNotFound && (strings.Contains(message, "凭据不存在") || errorTypeLower == "not_found") {
		return false, ""
	}
	if isModelForbiddenRejection(all) {
		return true, firstNonEmpty(message, text)
	}
	if status == http.StatusTooManyRequests || status >= 500 {
		return false, ""
	}
	if containsAny(all, []string{"too many failures", "toomanyfailures", "quotaexceeded", "quota exceeded", "upstreamforbidden", "manual", "disabledreason"}) &&
		!containsAny(all, []string{"凭证已被封禁或禁用", "temporarily_suspended", "accessdeniedexception", "validationexception", "resourcenotfoundexception"}) {
		return false, ""
	}

	if topReason != "" || reasonLower != "" {
		return true, firstNonEmpty(topReason, "上游返回永久拒绝 reason")
	}
	permanentNeedles := []string{
		"凭证已被封禁或禁用",
		"temporarily_suspended",
		"accessdeniedexception",
		"validationexception",
		"resourcenotfoundexception",
		"认证失败，token 无效或已过期",
		"expiredtoken",
		"unauthorizedexception",
		"oauth/idc 凭证已过期或无效",
		"idc 凭证已过期或无效",
		"oauth 凭证已过期或无效",
		"权限不足，无法刷新 token",
	}
	if containsAny(all, permanentNeedles) {
		return true, firstNonEmpty(message, text)
	}
	if (status == http.StatusBadRequest || status == http.StatusForbidden) && containsAny(messageLower, []string{"token 无效", "token 已过期", "凭证已过期", "凭据无效"}) {
		return true, firstNonEmpty(message, text)
	}
	return false, ""
}

func isModelForbiddenRejection(text string) bool {
	lower := strings.ToLower(text)
	if !strings.Contains(lower, strings.ToLower("ListAvailableModels")) &&
		!strings.Contains(lower, strings.ToLower("模型列表")) &&
		!strings.Contains(lower, "models") {
		return false
	}
	return strings.Contains(lower, "http 403") ||
		strings.Contains(lower, "403 forbidden") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "upstreamforbidden")
}

func parseKiroRSError(body []byte) (message string, errorType string, topReason string) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", ""
	}
	if reason, _ := payload["reason"].(string); strings.TrimSpace(reason) != "" {
		topReason = strings.TrimSpace(reason)
	}
	if msg, _ := payload["message"].(string); strings.TrimSpace(msg) != "" {
		message = strings.TrimSpace(msg)
	}
	if errObj, ok := payload["error"].(map[string]interface{}); ok {
		if msg, _ := errObj["message"].(string); strings.TrimSpace(msg) != "" {
			message = strings.TrimSpace(msg)
		}
		if typ, _ := errObj["type"].(string); strings.TrimSpace(typ) != "" {
			errorType = strings.TrimSpace(typ)
		}
	}
	return message, errorType, topReason
}

func containsAny(haystack string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "凭据永久失效"
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
