# kiro.rs 凭据同步集成设计

## 概述

为 kirox 桌面应用增加与 kiro.rs 服务的集成能力，实现注册成功的账号自动/手动同步到 kiro.rs 的凭据管理系统。

## 需求

1. 批量注册任务完成后，自动将本批次成功账号推送到 kiro.rs 的 `POST /api/admin/credentials` 接口
2. 账号池页面新增"同步到 kiro.rs"按钮，点击时全量推送所有账号
3. 在设置页面配置 kiro.rs 的 API 地址、API Key 和自动同步开关

## 设计决策

| 决策点 | 选择 | 理由 |
|---|---|---|
| 同步触发时机 | 整个批量任务完成后一次性同步 | 简单可靠，避免逐条调用的网络开销 |
| 失败处理 | 弹窗通知 + 自动重试一次 | 平衡用户感知与自动化 |
| 同步范围 | 全量推送，kiro.rs 侧去重 | 最简单，无需本地维护同步状态 |
| 自动同步控制 | 设置页开关 | 用户可控 |
| 架构方案 | Go 后端直接调用 | 符合现有项目模式，后端驱动业务逻辑 |

## 架构

```
┌─────────────────────────────────────────────────────┐
│ kirox 桌面应用                                       │
│                                                     │
│  ┌──────────┐    ┌──────────────┐    ┌───────────┐ │
│  │ Frontend │◄──►│   app.go     │◄──►│  storage  │ │
│  │ (JS)     │    │  (Wails绑定) │    │  (配置)   │ │
│  └──────────┘    └──────┬───────┘    └───────────┘ │
│                         │                           │
│  ┌──────────────────────┼───────────────────┐      │
│  │ coordinator.go       │                   │      │
│  │ (任务完成后触发同步) ▼                   │      │
│  │              ┌──────────────┐            │      │
│  │              │ kirorsync    │            │      │
│  │              │ (同步模块)   │            │      │
│  │              └──────┬───────┘            │      │
│  └─────────────────────┼───────────────────-┘      │
│                         │                           │
└─────────────────────────┼───────────────────────────┘
                          │ HTTP POST
                          ▼
              ┌───────────────────────┐
              │ kiro.rs 服务           │
              │ POST /api/admin/creds │
              │ Header: x-api-key     │
              └───────────────────────┘
```

## 模块设计

### 1. 存储层 (`internal/storage/storage.go`)

新增配置项：

| Key | 类型 | 说明 |
|---|---|---|
| `kiro_rs_api_url` | string | kiro.rs API 地址 |
| `kiro_rs_api_key` | string | Admin API Key |
| `kiro_rs_auto_sync` | bool | 注册完成后自动同步开关 |

方法签名：
- `GetKiroRSAPIURL() string`
- `SetKiroRSAPIURL(url string)`
- `GetKiroRSAPIKey() string`
- `SetKiroRSAPIKey(key string)`
- `GetKiroRSAutoSync() bool`
- `SetKiroRSAutoSync(enabled bool)`

### 2. 同步模块 (`internal/kirorsync/sync.go`)（新建）

```go
package kirorsync

type SyncResult struct {
    Total   int          `json:"total"`
    Success int          `json:"success"`
    Failed  int          `json:"failed"`
    Error   string       `json:"error,omitempty"` // 顶层错误（如"同步正在进行中"）
    Details []SyncDetail `json:"details"`
}

type SyncDetail struct {
    Email        string `json:"email"`
    Success      bool   `json:"success"`
    CredentialID int    `json:"credentialId,omitempty"`
    Error        string `json:"error,omitempty"`
}

// SyncAccounts 将账号列表逐条推送到 kiro.rs，失败项自动重试一次。
func SyncAccounts(apiURL, apiKey string, accounts []map[string]interface{}) SyncResult

// TestConnection 测试 kiro.rs 连通性和认证有效性（调用 GET /api/admin/credentials）。
func TestConnection(apiURL, apiKey string) error
```

**并发保护：** 模块内部维护一个 `sync.Mutex`，防止自动同步和手动同步并发执行。若已有同步在进行中，后续调用立即返回 `SyncResult{Error: "同步正在进行中"}`。

字段映射：

| kirox 账号字段 | kiro.rs AddCredentialRequest | 说明 |
|---|---|---|
| refreshToken | refreshToken | 必填 |
| clientId | clientId | BuilderId 认证需要 |
| clientSecret | clientSecret | BuilderId 认证需要 |
| region | authRegion | 默认 "us-east-1" |
| provider ("BuilderId") | authMethod = "idc" | 固定映射 |
| priority | priority | 数字越小优先级越高 |
| email | email | 随凭据同步传递 |
| password | — | 不传递（kiro.rs 不需要） |
| accessToken | — | 不传递（kiro.rs 自行刷新） |

流程：
1. 遍历账号，构造请求体（跳过 refreshToken 为空的账号）
2. 逐条 POST 到 `{apiURL}/api/admin/credentials`（kiro.rs 无批量接口）
3. 单条超时 10s；收集失败项（仅网络错误和 5xx 触发重试，401/400 不重试）
4. 对可重试失败项等待 2s 后统一重试一次
5. 返回 SyncResult

### 3. 任务协调器 (`internal/task/coordinator.go`)

**事件通知机制：** `task` 包不依赖 Wails runtime。在 `Manager` 上注入一个回调函数 `OnSyncResult func(SyncResult)`，由 `app.go` 在应用启动时设置，回调内部调用 `wailsRuntime.EventsEmit`。

在 `runBatch` 函数末尾（统计日志打印后）插入：

```go
if storage.GetKiroRSAutoSync() && storage.GetKiroRSAPIURL() != "" {
    accounts, _ := data.LoadAccounts(outDir)  // 本批次成功账号
    if len(accounts) > 0 {
        log.Printf("[Kiro] 开始自动同步 %d 个账号到 kiro.rs", len(accounts))
        result := kirorsync.SyncAccounts(
            storage.GetKiroRSAPIURL(),
            storage.GetKiroRSAPIKey(),
            accounts,
        )
        log.Printf("[Kiro] kiro.rs 同步完成: 成功 %d / 失败 %d", result.Success, result.Failed)
        if Manager.OnSyncResult != nil {
            Manager.OnSyncResult(result)
        }
    }
}
```

**数据源说明：**
- 自动同步：`data.LoadAccounts(outDir)` — 当前批次输出目录的 `accounts.json`（仅本批次成功账号）
- 手动同步：`data.LoadAccounts(storage.GetResultOutputDir())` — 全量账号池（所有历史成功账号）
- 两者数据结构一致，均为 `SaveKiroSuccess` 写入的 JSON 数组

### 4. App 绑定层 (`app.go`)

新增方法：

```go
// GetKiroRSConfig 获取 kiro.rs 同步配置
func (a *App) GetKiroRSConfig() map[string]interface{}

// SetKiroRSConfig 保存 kiro.rs 同步配置
func (a *App) SetKiroRSConfig(url, key string, autoSync bool) map[string]interface{}

// SyncAccountPoolToKiroRS 手动触发全量同步（账号池所有账号）
func (a *App) SyncAccountPoolToKiroRS() map[string]interface{}

// TestKiroRSConnection 测试 kiro.rs 连接
func (a *App) TestKiroRSConnection(url, key string) map[string]interface{}
```

在 `App.startup()` 中注入同步回调：
```go
task.Manager.OnSyncResult = func(result kirorsync.SyncResult) {
    wailsRuntime.EventsEmit(a.ctx, "kiro-rs-sync-result", result)
}
```

### 5. 前端

**task.js** — 监听同步结果事件：
```javascript
window.runtime.EventsOn('kiro-rs-sync-result', function(result) {
    var msg = 'kiro.rs 同步完成：成功 ' + result.success + ' / 失败 ' + result.failed;
    showToast(msg, result.failed > 0 ? 'error' : 'success');
});
```

**account_pool.js** — 新增同步按钮处理函数：
```javascript
async function syncAccountPoolToKiroRS() { ... }
```

按钮位置：账号池页面工具栏第一个按钮。

**设置页面** — 新增 "kiro.rs 同步" 配置区块：
- API 地址输入框
- API Key 输入框（password 类型）
- "测试连接"按钮（调用 `TestKiroRSConnection`，验证连通性和认证）
- 自动同步开关

## 错误处理

- API 地址为空时：自动同步静默跳过，手动同步返回错误提示
- API Key 无效时：kiro.rs 返回 401，记录到 SyncDetail.Error，不重试
- 网络超时/5xx：单条请求 10s 超时，失败后进入重试队列（等待 2s 后重试一次）
- 400 错误（参数无效）：不重试，直接记录失败
- refreshToken 重复：kiro.rs 侧去重处理，kirox 视为成功
- 并发保护：同步模块内部互斥，同一时刻只允许一个同步任务执行

## 安全考虑

- API Key 明文存储在本地配置文件中（与现有代理密码、Clash API Secret 等敏感信息同级，本地桌面应用可接受）
- 前端设置页面 API Key 输入框使用 password 类型
- HTTP 请求不经过外部代理（直连 kiro.rs 内网地址）
