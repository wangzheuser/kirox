# kiro.rs 凭据同步集成 实施计划

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 kirox 增加与 kiro.rs 服务的凭据同步能力，支持注册后自动同步和手动全量同步。

**Architecture:** Go 后端新增 `internal/kirorsync` 包封装 HTTP 调用逻辑；存储层新增 3 个配置项；coordinator 任务完成后通过回调通知前端；前端监听事件弹窗 + 账号池页面新增同步按钮。

**Tech Stack:** Go (Wails v2), vanilla JavaScript, HTTP REST API

---

## File Structure

| Action | Path | Responsibility |
|--------|------|----------------|
| Create | `internal/kirorsync/sync.go` | kiro.rs API 调用、字段映射、重试逻辑、并发保护 |
| Modify | `internal/storage/storage.go` | 新增 3 个配置项 getter/setter |
| Modify | `internal/task/state.go` | 添加 `OnSyncResult` 回调字段 |
| Modify | `internal/task/coordinator.go` | runBatch 末尾插入自动同步逻辑 |
| Modify | `app.go` | 新增 4 个 Wails 绑定方法 + startup 注入回调 |
| Modify | `frontend/index.html` | 设置页新增 kiro.rs 配置区块 + 账号池按钮 |
| Modify | `frontend/js/app.js` | 设置页加载/保存逻辑 |
| Modify | `frontend/js/account_pool.js` | 同步按钮处理函数 |
| Modify | `frontend/js/task.js` | 监听 kiro-rs-sync-result 事件 |

---

## Task 1: 存储层 — 新增 kiro.rs 配置项

**Files:**
- Modify: `internal/storage/storage.go`

- [ ] **Step 1: 添加常量和变量声明**

在 `storage.go` 的 `const` 块（约 line 42-69）末尾添加：

```go
keyKiroRSAPIURL   = "kiro_rs_api_url"
keyKiroRSAPIKey   = "kiro_rs_api_key"
keyKiroRSAutoSync = "kiro_rs_auto_sync"
```

在 `configKeyOrder` 切片（约 line 105-133）末尾添加这三个 key。

- [ ] **Step 2: 实现 getter/setter 方法**

在 `SetSoundEnabled` 函数之后（约 line 669）添加：

```go
// GetKiroRSAPIURL 返回 kiro.rs API 地址。
func GetKiroRSAPIURL() string {
	m := loadConfigMap()
	return strings.TrimSpace(m[keyKiroRSAPIURL])
}

// SetKiroRSAPIURL 保存 kiro.rs API 地址。
func SetKiroRSAPIURL(url string) error {
	return modifyConfigMap(func(m map[string]string) error {
		m[keyKiroRSAPIURL] = strings.TrimSpace(url)
		return nil
	})
}

// GetKiroRSAPIKey 返回 kiro.rs Admin API Key。
func GetKiroRSAPIKey() string {
	m := loadConfigMap()
	return strings.TrimSpace(m[keyKiroRSAPIKey])
}

// SetKiroRSAPIKey 保存 kiro.rs Admin API Key。
func SetKiroRSAPIKey(key string) error {
	return modifyConfigMap(func(m map[string]string) error {
		m[keyKiroRSAPIKey] = strings.TrimSpace(key)
		return nil
	})
}

// GetKiroRSAutoSync 返回注册完成后是否自动同步到 kiro.rs。
func GetKiroRSAutoSync() bool {
	m := loadConfigMap()
	return parseBool(m[keyKiroRSAutoSync])
}

// SetKiroRSAutoSync 保存自动同步开关状态。
func SetKiroRSAutoSync(enabled bool) error {
	return modifyConfigMap(func(m map[string]string) error {
		m[keyKiroRSAutoSync] = strconv.FormatBool(enabled)
		return nil
	})
}
```

- [ ] **Step 3: 验证编译通过**

Run: `cd D:/projects/github/kirox && go build ./internal/storage/...`
Expected: 无错误输出

---

## Task 2: 同步模块 — 创建 `internal/kirorsync/sync.go`

**Files:**
- Create: `internal/kirorsync/sync.go`

- [ ] **Step 1: 创建同步模块文件**

```go
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
	syncMu   sync.Mutex
	syncing  bool
	client   = &http.Client{Timeout: 10 * time.Second}
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
		return SyncResult{Total: 0, Success: 0, Failed: 0}
	}

	result := SyncResult{Total: len(validAccounts)}
	var retryable []map[string]interface{}

	// 第一轮推送
	for _, acc := range validAccounts {
		detail := pushOne(apiURL, apiKey, acc)
		if detail.Success {
			result.Success++
		} else {
			// 仅网络错误和 5xx 可重试
			if isRetryableError(detail.Error) {
				retryable = append(retryable, acc)
			} else {
				result.Failed++
			}
		}
		result.Details = append(result.Details, detail)
	}

	// 重试轮：等待 2s 后统一重试
	if len(retryable) > 0 {
		log.Printf("[Kiro] kiro.rs 同步重试: %d 条失败记录", len(retryable))
		time.Sleep(2 * time.Second)
		for i, acc := range retryable {
			detail := pushOne(apiURL, apiKey, acc)
			// 更新对应的 detail（找到同 email 的失败记录替换）
			email, _ := acc["email"].(string)
			for j := range result.Details {
				if result.Details[j].Email == email && !result.Details[j].Success {
					result.Details[j] = detail
					break
				}
			}
			if detail.Success {
				result.Success++
			} else {
				result.Failed++
			}
			_ = i
		}
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
	url := strings.TrimRight(apiURL, "/") + "/api/admin/credentials"
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
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

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

- [ ] **Step 2: 验证编译通过**

Run: `cd D:/projects/github/kirox && go build ./internal/kirorsync/...`
Expected: 无错误输出

---

## Task 3: 任务状态 — 添加同步回调字段

**Files:**
- Modify: `internal/task/state.go:14-28`

- [ ] **Step 1: 在 State 结构体中添加回调字段**

在 `State` 结构体的 `logFile` 字段之后添加：

```go
// OnSyncResult 同步完成回调（由 app.go 注入，用于通知前端）
OnSyncResult func(interface{})
```

注意：使用 `func(interface{})` 而非直接引用 `kirorsync.SyncResult`，避免 `task` 包对 `kirorsync` 包的循环依赖。

- [ ] **Step 2: 验证编译通过**

Run: `cd D:/projects/github/kirox && go build ./internal/task/...`
Expected: 无错误输出

---

## Task 4: 协调器 — 插入自动同步逻辑

**Files:**
- Modify: `internal/task/coordinator.go:688-692`

- [ ] **Step 1: 添加 import**

在 `coordinator.go` 的 import 块中添加：

```go
"reg_go/internal/kirorsync"
```

- [ ] **Step 2: 在 runBatch 末尾插入同步逻辑**

在 `log.Println("[Kiro] ═══════════════════════════════")` 最后一行（约 line 691）之后、`}` 函数结束之前插入：

```go

	// 自动同步到 kiro.rs
	if sucCount > 0 && storage.GetKiroRSAutoSync() && storage.GetKiroRSAPIURL() != "" {
		accounts, _ := data.LoadAccounts(outDir)
		if len(accounts) > 0 {
			log.Printf("[Kiro] 开始自动同步 %d 个账号到 kiro.rs", len(accounts))
			syncResult := kirorsync.SyncAccounts(
				storage.GetKiroRSAPIURL(),
				storage.GetKiroRSAPIKey(),
				accounts,
			)
			log.Printf("[Kiro] kiro.rs 同步完成: 成功 %d / 失败 %d", syncResult.Success, syncResult.Failed)
			if Manager.OnSyncResult != nil {
				Manager.OnSyncResult(syncResult)
			}
		}
	}
```

- [ ] **Step 3: 验证编译通过**

Run: `cd D:/projects/github/kirox && go build ./internal/task/...`
Expected: 无错误输出

---

## Task 5: App 绑定层 — 新增 Wails 方法

**Files:**
- Modify: `app.go`

- [ ] **Step 1: 添加 import**

在 `app.go` 的 import 块中添加：

```go
"reg_go/internal/kirorsync"
```

- [ ] **Step 2: 在 startup 中注入同步回调**

在 `startup` 函数的 `go updater.CleanupTemp()` 之后添加：

```go
	// 注入 kiro.rs 同步完成回调
	task.Manager.OnSyncResult = func(result interface{}) {
		runtime.EventsEmit(ctx, "kiro-rs-sync-result", result)
	}
```

- [ ] **Step 3: 添加配置读写方法**

在 `ExportAccountPoolJSON` 方法之后添加：

```go
// GetKiroRSConfig 获取 kiro.rs 同步配置
func (a *App) GetKiroRSConfig() map[string]interface{} {
	return map[string]interface{}{
		"apiURL":   storage.GetKiroRSAPIURL(),
		"apiKey":   storage.GetKiroRSAPIKey(),
		"autoSync": storage.GetKiroRSAutoSync(),
	}
}

// SetKiroRSConfig 保存 kiro.rs 同步配置
func (a *App) SetKiroRSConfig(url, key string, autoSync bool) map[string]interface{} {
	if err := storage.SetKiroRSAPIURL(url); err != nil {
		return map[string]interface{}{"error": "保存 API 地址失败: " + err.Error()}
	}
	if err := storage.SetKiroRSAPIKey(key); err != nil {
		return map[string]interface{}{"error": "保存 API Key 失败: " + err.Error()}
	}
	if err := storage.SetKiroRSAutoSync(autoSync); err != nil {
		return map[string]interface{}{"error": "保存自动同步设置失败: " + err.Error()}
	}
	return map[string]interface{}{"success": true}
}

// TestKiroRSConnection 测试 kiro.rs 连接
func (a *App) TestKiroRSConnection(url, key string) map[string]interface{} {
	if url == "" {
		return map[string]interface{}{"success": false, "error": "请输入 API 地址"}
	}
	if key == "" {
		return map[string]interface{}{"success": false, "error": "请输入 API Key"}
	}
	if err := kirorsync.TestConnection(url, key); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "message": "连接成功"}
}

// SyncAccountPoolToKiroRS 手动触发全量同步到 kiro.rs
func (a *App) SyncAccountPoolToKiroRS() map[string]interface{} {
	apiURL := storage.GetKiroRSAPIURL()
	apiKey := storage.GetKiroRSAPIKey()
	if apiURL == "" {
		return map[string]interface{}{"error": "请先配置 kiro.rs API 地址"}
	}
	if apiKey == "" {
		return map[string]interface{}{"error": "请先配置 kiro.rs API Key"}
	}

	accounts, err := data.LoadAccounts(storage.GetResultOutputDir())
	if err != nil {
		return map[string]interface{}{"error": "加载账号池失败: " + err.Error()}
	}
	if len(accounts) == 0 {
		return map[string]interface{}{"error": "账号池为空，无可同步账号"}
	}

	result := kirorsync.SyncAccounts(apiURL, apiKey, accounts)
	if result.Error != "" {
		return map[string]interface{}{"error": result.Error}
	}
	return map[string]interface{}{
		"success": true,
		"total":   result.Total,
		"success": result.Success,
		"failed":  result.Failed,
	}
}
```

注意：`SyncAccountPoolToKiroRS` 的返回 map 中有两个 `"success"` key，需要修正。改为：

```go
	return map[string]interface{}{
		"success":     true,
		"total":       result.Total,
		"syncSuccess": result.Success,
		"syncFailed":  result.Failed,
	}
```

- [ ] **Step 4: 验证编译通过**

Run: `cd D:/projects/github/kirox && go build .`
Expected: 无错误输出

---

## Task 6: 前端 — 设置页面 kiro.rs 配置区块

**Files:**
- Modify: `frontend/index.html:561` (在通知 card 之后、`</div>` page-scroll 结束之前)
- Modify: `frontend/js/app.js`

- [ ] **Step 1: 在 index.html 设置页面添加 kiro.rs 配置区块**

在 line 561（通知 settings-group 的 `</div></div>` 之后）插入：

```html
        <!-- kiro.rs 同步 -->
        <div class="card" style="margin-bottom:20px;">
          <div class="settings-group">
            <div class="settings-group-title">kiro.rs 同步</div>

            <div class="settings-item">
              <div class="settings-item-main">
                <div class="settings-item-title">API 地址</div>
                <div class="settings-item-desc">kiro.rs 服务的访问地址</div>
              </div>
              <div class="settings-item-action" style="min-width:320px;">
                <input type="text" id="cfg-kiro-rs-url" placeholder="http://host:port" class="form-input" style="width:100%;">
              </div>
            </div>

            <div class="settings-item">
              <div class="settings-item-main">
                <div class="settings-item-title">API Key</div>
                <div class="settings-item-desc">kiro.rs Admin API 认证密钥</div>
              </div>
              <div class="settings-item-action" style="min-width:320px;">
                <input type="password" id="cfg-kiro-rs-key" placeholder="Admin API Key" class="form-input" style="width:100%;">
              </div>
            </div>

            <div class="settings-item">
              <div class="settings-item-main">
                <div class="settings-item-title">注册后自动同步</div>
                <div class="settings-item-desc">批量注册任务完成后自动将成功账号推送到 kiro.rs</div>
              </div>
              <div class="settings-item-action">
                <label style="cursor:pointer;">
                  <div class="toggle-switch">
                    <input type="checkbox" id="cfg-kiro-rs-auto-sync">
                    <span class="toggle-slider"></span>
                  </div>
                </label>
              </div>
            </div>

            <div class="settings-item">
              <div class="settings-item-main" style="flex:0;">
              </div>
              <div class="settings-item-action" style="display:flex;gap:8px;">
                <button onclick="testKiroRSConnection()" class="btn btn-secondary btn-sm">测试连接</button>
                <button onclick="saveKiroRSConfig()" class="btn btn-dark btn-sm">保存</button>
              </div>
            </div>
          </div>
        </div>
```

- [ ] **Step 2: 在 app.js 中添加加载和保存函数**

在 `saveKillSwitchEnabled` 函数之后添加：

```javascript
// ===== kiro.rs 同步配置 =====

async function loadKiroRSConfig() {
  try {
    var cfg = await window.go.main.App.GetKiroRSConfig();
    var urlEl = document.getElementById('cfg-kiro-rs-url');
    var keyEl = document.getElementById('cfg-kiro-rs-key');
    var autoEl = document.getElementById('cfg-kiro-rs-auto-sync');
    if (urlEl) urlEl.value = cfg.apiURL || '';
    if (keyEl) keyEl.value = cfg.apiKey || '';
    if (autoEl) autoEl.checked = !!cfg.autoSync;
  } catch(e) {}
}

async function saveKiroRSConfig() {
  var url = (document.getElementById('cfg-kiro-rs-url') || {}).value || '';
  var key = (document.getElementById('cfg-kiro-rs-key') || {}).value || '';
  var autoSync = !!(document.getElementById('cfg-kiro-rs-auto-sync') || {}).checked;
  try {
    var result = await window.go.main.App.SetKiroRSConfig(url.trim(), key.trim(), autoSync);
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    showToast('kiro.rs 配置已保存');
  } catch(e) {
    showToast('保存失败: ' + e.message, 'error');
  }
}

async function testKiroRSConnection() {
  var url = (document.getElementById('cfg-kiro-rs-url') || {}).value || '';
  var key = (document.getElementById('cfg-kiro-rs-key') || {}).value || '';
  try {
    var result = await window.go.main.App.TestKiroRSConnection(url.trim(), key.trim());
    if (result.success) {
      showToast('连接成功', 'success');
    } else {
      showToast(result.error || '连接失败', 'error');
    }
  } catch(e) {
    showToast('测试失败: ' + e.message, 'error');
  }
}
```

- [ ] **Step 3: 在页面初始化时调用 loadKiroRSConfig**

找到 `app.js` 中页面初始化逻辑（DOMContentLoaded 或 init 函数），添加 `loadKiroRSConfig()` 调用。如果现有设置加载在 `switchPage('settings')` 时触发，则在对应位置添加。

- [ ] **Step 4: 验证页面渲染正常**

Run: `cd D:/projects/github/kirox && go build . && echo "BUILD OK"`
Expected: BUILD OK

---

## Task 7: 前端 — 账号池同步按钮

**Files:**
- Modify: `frontend/index.html:652-653`
- Modify: `frontend/js/account_pool.js`

- [ ] **Step 1: 在账号池工具栏添加同步按钮**

在 `frontend/index.html` line 652-653 的按钮行中，在 `<button onclick="openAccountPoolImportModal()"` 之前插入：

```html
              <button id="btn-sync-kiro-rs" onclick="syncAccountPoolToKiroRS()" class="btn btn-dark btn-sm">同步到 kiro.rs</button>
```

- [ ] **Step 2: 在 account_pool.js 末尾添加同步函数**

```javascript
async function syncAccountPoolToKiroRS() {
  var btn = document.getElementById('btn-sync-kiro-rs');
  if (btn) btn.disabled = true;
  try {
    var result = await window.go.main.App.SyncAccountPoolToKiroRS();
    if (result.error) {
      showToast(result.error, 'error');
      return;
    }
    var msg = 'kiro.rs 同步完成：成功 ' + result.syncSuccess + ' / 失败 ' + result.syncFailed;
    showToast(msg, result.syncFailed > 0 ? 'error' : 'success');
  } catch (e) {
    showToast('同步失败: ' + e.message, 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}
```

---

## Task 8: 前端 — 监听自动同步事件

**Files:**
- Modify: `frontend/js/task.js:236-244`

- [ ] **Step 1: 在 task.js 的事件注册区域添加监听**

在 `window.runtime.EventsOn('update-progress', ...)` 之后（约 line 243）添加：

```javascript
  window.runtime.EventsOn('kiro-rs-sync-result', function(result) {
    if (result.error) {
      showToast('kiro.rs 同步失败: ' + result.error, 'error');
      return;
    }
    var msg = 'kiro.rs 同步完成：成功 ' + result.success + ' / 失败 ' + result.failed;
    showToast(msg, result.failed > 0 ? 'error' : 'success');
  });
```

---

## Task 9: 最终验证

- [ ] **Step 1: 完整编译验证**

Run: `cd D:/projects/github/kirox && go build .`
Expected: 无错误输出

- [ ] **Step 2: 运行现有测试**

Run: `cd D:/projects/github/kirox && go test ./...`
Expected: 所有测试通过（或与本次变更无关的已知失败）

- [ ] **Step 3: 前端构建验证**

Run: `cd D:/projects/github/kirox/frontend && node build.js`
Expected: 构建成功
