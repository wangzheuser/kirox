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
	Email        string `json:"email"`
	Success      bool   `json:"success"`
	CredentialID int    `json:"credentialId,omitempty"`
	Error        string `json:"error,omitempty"`
}

// addCredentialRequest kiro.rs 添加凭据请求体
type addCredentialRequest struct {
	RefreshToken string `json:"refreshToken"`
	AuthMethod   string `json:"authMethod,omitempty"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Priority     int    `json:"priority,omitempty"`
	AuthRegion   string `json:"authRegion,omitempty"`
}

// addCredentialResponse kiro.rs 添加凭据响应体
type addCredentialResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	CredentialID int    `json:"credentialId"`
	Email        string `json:"email"`
}

var (
	syncMu  sync.Mutex
	syncing bool
	client  = &http.Client{Timeout: 10 * time.Second}
)

// SyncAccounts 将账号列表逐条推送到 kiro.rs，失败项自动重试一次。
// 并发保护：同一时刻只允许一个同步任务执行。
func SyncAccounts(apiURL, apiKey string, accounts []map[string]interface{}) SyncResult {
	syncMu.Lock()
	if syncing {
		syncMu.Unlock()
		return SyncResult{Error: "同步正在进行中"}
	}
	syncing = true
	syncMu.Unlock()
	defer func() {
		syncMu.Lock()
		syncing = false
		syncMu.Unlock()
	}()

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
		if !detail.Success && isRetryableError(detail.Error) {
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
		return SyncDetail{Email: email, Success: false, Error: errMsg}
	}

	var respData addCredentialResponse
	if err := json.Unmarshal(respBody, &respData); err != nil {
		// 状态码 2xx 但解析失败，仍视为成功
		return SyncDetail{Email: email, Success: true}
	}

	return SyncDetail{
		Email:        email,
		Success:      true,
		CredentialID: respData.CredentialID,
	}
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

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
