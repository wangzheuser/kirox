package main

import (
	"context"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"log"
	"os"
	"reg_go/internal/data"
	"reg_go/internal/email"
	"reg_go/internal/kirorsync"
	"reg_go/internal/proxy"
	"reg_go/internal/subscription"

	"reg_go/internal/storage"
	"reg_go/internal/task"
	"reg_go/internal/updater"
	"time"
)

type App struct {
	ctx context.Context
}

// NewApp 创建新的 App 实例
func NewApp() *App {
	return &App{}
}

// startup 在应用启动时调用
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// 重定向日志到内存
	log.SetOutput(&logWriter{app: a})
	log.SetFlags(log.Ltime)

	// 居中显示窗口
	go func() {
		time.Sleep(200 * time.Millisecond)
		runtime.WindowCenter(ctx)
	}()

	// 清理上次更新可能遗留的临时文件
	go updater.CleanupTemp()

	// 注入 kiro.rs 同步完成回调
	task.Manager.OnSyncResult = func(result interface{}) {
		runtime.EventsEmit(ctx, "kiro-rs-sync-result", result)
	}
}

// shutdown 在应用关闭时调用
func (a *App) shutdown(ctx context.Context) {
	storage.FlushAccountsSync()
}

// OpenURL 在系统默认浏览器中打开 URL
func (a *App) OpenURL(url string) {
	runtime.BrowserOpenURL(a.ctx, url)
}

// logWriter 自定义日志写入器，根据运行状态路由日志
type logWriter struct {
	app *App
}

func (w *logWriter) Write(p []byte) (int, error) {
	msg := string(p)
	task.Manager.AppendLog(msg)
	return os.Stderr.Write(p)
}

// GetStatus 获取任务状态
func (a *App) GetStatus() map[string]interface{} {
	return task.Manager.GetStatus()
}

// GetLogs 获取日志
func (a *App) GetLogs() []string {
	return task.Manager.GetLogs()
}

// GetOverview 获取全局概览数据
func (a *App) GetOverview() map[string]interface{} {
	// Outlook 账号统计
	outlookTotal, outlookRegistered, outlookSuccess, outlookPending := countOutlookAccounts()

	// 当前任务状态
	taskStatus := task.Manager.GetStatus()

	return map[string]interface{}{
		"version": updater.GetCurrentVersion(),
		"kiro": map[string]interface{}{
			"taskRunning":   taskStatus["running"],
			"taskSuccess":   taskStatus["success"],
			"taskFailed":    taskStatus["failed"],
			"taskCompleted": taskStatus["completed"],
			"taskTotal":     taskStatus["total"],
		},
		"outlook": map[string]interface{}{
			"total":      outlookTotal,
			"registered": outlookRegistered,
			"success":    outlookSuccess,
			"pending":    outlookPending,
		},
	}
}

// GetTaskStatus 获取实时任务状态
func (a *App) GetTaskStatus() map[string]interface{} {
	taskStatus := task.Manager.GetStatus()
	return map[string]interface{}{
		"kiro": map[string]interface{}{
			"taskRunning":   taskStatus["running"],
			"taskSuccess":   taskStatus["success"],
			"taskFailed":    taskStatus["failed"],
			"taskCompleted": taskStatus["completed"],
			"taskTotal":     taskStatus["total"],
		},
	}
}

// countOutlookAccounts 统计 Outlook 账号
func countOutlookAccounts() (total, registered, success, pending int) {
	accounts := storage.GetAccountsCached()
	if len(accounts) == 0 {
		return
	}
	total = len(accounts)
	for _, acc := range accounts {
		reg, _ := acc["registered"].(bool)
		suc, _ := acc["success"].(bool)
		if reg {
			registered++
			if suc {
				success++
			}
		} else {
			pending++
		}
	}
	return
}

// VerifyLicense 验证卡密
func (a *App) VerifyLicense(licenseKey string) map[string]interface{} {
	return map[string]interface{}{"success": true}
}

// CheckLicense 检查本地卡密
func (a *App) CheckLicense() map[string]interface{} {
	return map[string]interface{}{"valid": true}
}

// GetLicenseInfo 获取卡密详细信息
func (a *App) GetLicenseInfo() map[string]interface{} {
	return map[string]interface{}{"success": true, "key": ""}
}

// LogoutLicense 退出卡密
func (a *App) LogoutLicense() map[string]interface{} {
	return map[string]interface{}{"success": true, "message": "已退出"}
}

// ---- MoeMail ----

func (a *App) GetMoeMailConfigs() []email.MoeMailConfig {
	return email.GetMoeMailConfigs()
}

func (a *App) SaveMoeMailConfigs(configsJSON string) map[string]interface{} {
	return email.SaveMoeMailConfigs(configsJSON)
}

func (a *App) TestMoeMailConnection(configJSON string) map[string]interface{} {
	return email.TestMoeMailConnection(configJSON)
}

// ---- Outlook ----

func (a *App) AddOutlookAccounts(data string) map[string]interface{} {
	return email.AddOutlookAccounts(data)
}

func (a *App) GetOutlookAccounts() []map[string]interface{} {
	return email.GetOutlookAccounts()
}

func (a *App) DeleteOutlookAccount(em string) map[string]interface{} {
	return email.DeleteOutlookAccount(em)
}

func (a *App) DeleteOutlookAccounts(emails []string) map[string]interface{} {
	return email.DeleteOutlookAccounts(emails)
}

func (a *App) ClearOutlookAccounts() map[string]interface{} {
	return email.ClearOutlookAccounts()
}

func (a *App) ClearRegisteredOutlookAccounts() map[string]interface{} {
	return email.ClearRegisteredOutlookAccounts()
}

// ResetOutlookAccountStatuses 重置所有 Outlook 账号状态但不删除账号。
func (a *App) ResetOutlookAccountStatuses() map[string]interface{} {
	return email.ResetOutlookAccountStatuses()
}

func (a *App) ImportOutlookFile(filePath string) map[string]interface{} {
	return email.ImportOutlookFile(filePath)
}

// ---- Wails 专用对话框 ----

// SelectDirectory 选择目录 (Wails Dialog)
func (a *App) SelectDirectory() string {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择目录",
	})
	if err != nil {
		log.Printf("选择目录失败: %v", err)
		return ""
	}
	return path
}

// SelectOutlookFile 选择 Outlook 账号文件 (Wails Dialog)
func (a *App) SelectOutlookFile() string {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Outlook 账号文件",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "文本文件 (*.txt)",
				Pattern:     "*.txt",
			},
			{
				DisplayName: "CSV 文件 (*.csv)",
				Pattern:     "*.csv",
			},
			{
				DisplayName: "所有文件 (*.*)",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		log.Printf("选择文件失败: %v", err)
		return ""
	}
	return path
}

// GetDataDir 前端获取当前存储目录
func (a *App) GetDataDir() string {
	return storage.GetDataDir()
}

// SetDataDir 设置自定义存储目录（自动迁移旧数据）
func (a *App) SetDataDir(dir string) map[string]interface{} {
	path, err := storage.SetDataDirPath(dir)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "path": path}
}

// ResetDataDir 重置为默认存储目录
func (a *App) ResetDataDir() map[string]interface{} {
	path := storage.ResetDataDirPath()
	return map[string]interface{}{"success": true, "path": path}
}

// GetResultOutputDir 获取注册结果输出目录（明文 accounts.json 的写入位置）
func (a *App) GetResultOutputDir() string {
	return storage.GetResultOutputDir()
}

// SetResultOutputDir 设置注册结果输出目录
func (a *App) SetResultOutputDir(dir string) map[string]interface{} {
	path, err := storage.SetResultOutputDir(dir)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "path": path}
}

// ResetResultOutputDir 重置为默认输出目录
func (a *App) ResetResultOutputDir() map[string]interface{} {
	path := storage.ResetResultOutputDir()
	return map[string]interface{}{"success": true, "path": path}
}

// GetPageStayConfig 获取发送验证码前模拟页面停留配置。
func (a *App) GetPageStayConfig() storage.PageStayConfig {
	return storage.GetPageStayConfig()
}

// SetPageStayConfig 保存发送验证码前模拟页面停留配置。
func (a *App) SetPageStayConfig(config storage.PageStayConfig) map[string]interface{} {
	if err := storage.SetPageStayConfig(config); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "config": storage.GetPageStayConfig()}
}

// GetOutlookScope 获取 Outlook 验证码读取方式。
func (a *App) GetOutlookScope() string {
	return storage.GetOutlookScope()
}

// SetOutlookScope 保存 Outlook 验证码读取方式。
func (a *App) SetOutlookScope(scope string) map[string]interface{} {
	if err := storage.SetOutlookScope(scope); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "scope": storage.GetOutlookScope()}
}

// GetProxy 返回当前全局代理（空字符串=直连）
func (a *App) GetProxy() string {
	return storage.GetProxy()
}

// SetProxy 保存全局代理；输入的简写（host:port:user:pass 等）会被自动归一化；
// 保存后会探测代理出口 IP 与归属信息并一并返回。
func (a *App) SetProxy(raw string) map[string]interface{} {
	normalized, err := storage.SetProxy(raw)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	resp := map[string]interface{}{"success": true, "proxy": normalized}
	if normalized != "" {
		resp["detect"] = proxy.Detect(normalized)
	}
	return resp
}

// DetectProxy 单独探测一个代理（不保存），用于"测试连接"
func (a *App) DetectProxy(raw string) proxy.Info {
	normalized := storage.NormalizeProxyAddress(raw)
	return proxy.Detect(normalized)
}

// GetEmailProxy 返回邮箱 API 专用代理（空字符串=直连）。
func (a *App) GetEmailProxy() string {
	return storage.GetEmailProxy()
}

// SetEmailProxy 保存邮箱 API 专用代理；留空表示直连。
func (a *App) SetEmailProxy(raw string) map[string]interface{} {
	normalized, err := storage.SetEmailProxy(raw)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	resp := map[string]interface{}{"success": true, "proxy": normalized}
	if normalized != "" {
		resp["detect"] = proxy.Detect(normalized)
	}
	return resp
}

// DetectEmailProxy 单独探测邮箱 API 专用代理（不保存）。
func (a *App) DetectEmailProxy(raw string) proxy.Info {
	normalized := storage.NormalizeProxyAddress(raw)
	return proxy.Detect(normalized)
}

// ResetEmailProxy 清空邮箱 API 专用代理，恢复直连。
func (a *App) ResetEmailProxy() map[string]interface{} {
	storage.ResetEmailProxy()
	return map[string]interface{}{"success": true}
}

// GetProxyMode 返回当前互斥代理模式。
func (a *App) GetProxyMode() string {
	return storage.GetProxyMode()
}

// SetProxyMode 保存当前互斥代理模式。
func (a *App) SetProxyMode(mode string) map[string]interface{} {
	if err := storage.SetProxyMode(mode); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "mode": storage.GetProxyMode()}
}

// GetClashProxy 返回 Clash 本地代理地址。
func (a *App) GetClashProxy() string {
	return storage.GetClashProxy()
}

// SetClashProxy 保存 Clash 本地代理地址。
func (a *App) SetClashProxy(raw string) map[string]interface{} {
	normalized, err := storage.SetClashProxy(raw)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "proxy": normalized}
}

// GetClashConfig 返回 Clash API 自动切换配置。
func (a *App) GetClashConfig() proxy.ClashConfig {
	return storage.GetClashConfig()
}

// SetClashConfig 保存 Clash API 自动切换配置。
func (a *App) SetClashConfig(config proxy.ClashConfig) map[string]interface{} {
	if err := storage.SetClashConfig(config); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "config": storage.GetClashConfig()}
}

// DetectClashProxy 先切换 Clash 节点，再检测本地代理出口。
func (a *App) DetectClashProxy(raw string, config proxy.ClashConfig) proxy.Info {
	normalized := storage.NormalizeProxyAddress(raw)
	return proxy.DetectClash(normalized, config)
}

// ResetProxy 清空代理，恢复直连
func (a *App) ResetProxy() map[string]interface{} {
	storage.ResetProxy()
	return map[string]interface{}{"success": true}
}

// GetKillSwitchEnabled 返回熔断级错误自动停止开关状态
func (a *App) GetKillSwitchEnabled() bool {
	return storage.GetKillSwitchEnabled()
}

// SetKillSwitchEnabled 保存熔断级错误自动停止开关状态
func (a *App) SetKillSwitchEnabled(enabled bool) map[string]interface{} {
	if err := storage.SetKillSwitchEnabled(enabled); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "enabled": enabled}
}

// GetSoundEnabled 返回任务结束提示音开关状态
func (a *App) GetSoundEnabled() bool {
	return storage.GetSoundEnabled()
}

// SetSoundEnabled 保存任务结束提示音开关状态
func (a *App) SetSoundEnabled(enabled bool) map[string]interface{} {
	if err := storage.SetSoundEnabled(enabled); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"success": true, "enabled": storage.GetSoundEnabled()}
}

// GetRegistrationConfig 返回注册页业务配置
func (a *App) GetRegistrationConfig() storage.RegistrationConfig {
	return storage.GetRegistrationConfig()
}

// SetRegistrationConfig 保存注册页业务配置
func (a *App) SetRegistrationConfig(config storage.RegistrationConfig) map[string]interface{} {
	if err := storage.SetRegistrationConfig(config); err != nil {
		return map[string]interface{}{"error": err.Error(), "config": storage.GetRegistrationConfig()}
	}
	return map[string]interface{}{"success": true, "config": storage.GetRegistrationConfig()}
}

// StartTask 启动注册任务
func (a *App) StartTask(req task.StartTaskRequest) map[string]interface{} {
	return task.StartTask(req)
}

// StopTask 停止注册任务
func (a *App) StopTask() map[string]interface{} {
	return task.StopTask(true)
}

// CheckUpdate 手动检查更新
func (a *App) CheckUpdate() map[string]interface{} {
	return updater.CheckUpdate()
}

// DownloadUpdate 下载更新（使用服务端缓存的下载地址，不接受前端参数）
func (a *App) DownloadUpdate() map[string]interface{} {
	return updater.DownloadUpdate(a.ctx)
}

// CancelUpdate 取消正在进行的更新下载
func (a *App) CancelUpdate() map[string]interface{} {
	return updater.CancelUpdate()
}

// ---- 订阅：一键获取支付链接 ----

func accountFromMap(m map[string]interface{}) subscription.Account {
	get := func(k string) string { v, _ := m[k].(string); return v }
	return subscription.Account{
		Email:        get("email"),
		RefreshToken: get("refreshToken"),
		ClientID:     get("clientId"),
		ClientSecret: get("clientSecret"),
		Region:       get("region"),
		Provider:     get("provider"),
		Time:         get("time"),
		Subscription: get("subscription"),
	}
}

// LoadOutputAccounts 读取当前输出目录下 accounts.json 中的账号列表，并附带已缓存的订阅链接信息
func (a *App) LoadOutputAccounts() map[string]interface{} {
	items, err := data.LoadAccounts(storage.GetResultOutputDir())
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	cache := subscription.LoadCache(storage.GetDataDir())
	for _, m := range items {
		if em, _ := m["email"].(string); em != "" {
			if entry, ok := cache[em]; ok {
				m["cachedUrl"] = entry.URL
				m["cachedPlanType"] = entry.PlanType
				m["cachedFetchedAt"] = entry.FetchedAt
			}
		}
	}
	return map[string]interface{}{"success": true, "accounts": items, "outputDir": storage.GetResultOutputDir()}
}

// ListAccountPool 读取订阅账号池（复用结果输出目录 accounts.json）。
func (a *App) ListAccountPool() map[string]interface{} {
	items, err := data.ListAccountPool(storage.GetResultOutputDir())
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error(), "outputDir": storage.GetResultOutputDir()}
	}
	return map[string]interface{}{"success": true, "accounts": items, "outputDir": storage.GetResultOutputDir()}
}

// ImportAccountPoolJSON 导入参考插件导出的账号 JSON。
func (a *App) ImportAccountPoolJSON(raw string) map[string]interface{} {
	summary, err := data.ImportAccountPoolJSON(storage.GetResultOutputDir(), raw)
	if err != nil {
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
			"summary": summary,
		}
	}
	return map[string]interface{}{
		"success":  true,
		"imported": summary.Imported,
		"updated":  summary.Updated,
		"skipped":  summary.Skipped,
		"total":    summary.Total,
		"summary":  summary,
	}
}

// ExportAccountPoolJSON 导出参考插件兼容的账号 JSON。
func (a *App) ExportAccountPoolJSON() map[string]interface{} {
	jsonText, count, err := data.ExportAccountPoolJSON(storage.GetResultOutputDir())
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{
		"success":   true,
		"data":      jsonText,
		"json":      jsonText,
		"count":     count,
		"outputDir": storage.GetResultOutputDir(),
	}
}

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

	log.Printf("[Kiro] 手动同步账号池到 kiro.rs：共加载 %d 个账号", len(accounts))
	result := kirorsync.SyncAccounts(apiURL, apiKey, accounts)
	if result.Error != "" {
		log.Printf("[Kiro] 手动同步未执行: %s", result.Error)
		return map[string]interface{}{"error": result.Error}
	}
	log.Printf("[Kiro] 手动同步完成: 成功 %d / 失败 %d", result.Success, result.Failed)
	return map[string]interface{}{
		"success":     true,
		"total":       result.Total,
		"syncSuccess": result.Success,
		"syncFailed":  result.Failed,
	}
}

// GetSubscriptionPlans 用第一个有效账号拉取可用订阅计划（可指定邮箱）
func (a *App) GetSubscriptionPlans(email string) map[string]interface{} {
	items, err := data.LoadAccounts(storage.GetResultOutputDir())
	if err != nil || len(items) == 0 {
		return map[string]interface{}{"success": false, "error": "未找到任何账号"}
	}
	// 如指定邮箱，优先用该账号
	if email != "" {
		for _, m := range items {
			if e, _ := m["email"].(string); e == email {
				acc := accountFromMap(m)
				token, err := subscription.RefreshAccessToken(acc)
				if err != nil {
					return map[string]interface{}{"success": false, "error": err.Error()}
				}
				plans, err := subscription.ListPlans(acc, token)
				if err != nil {
					return map[string]interface{}{"success": false, "error": err.Error()}
				}
				return map[string]interface{}{"success": true, "plans": plans}
			}
		}
		return map[string]interface{}{"success": false, "error": "未找到账号: " + email}
	}
	var lastErr error
	for _, m := range items {
		acc := accountFromMap(m)
		if acc.RefreshToken == "" || acc.ClientID == "" {
			continue
		}
		token, err := subscription.RefreshAccessToken(acc)
		if err != nil {
			lastErr = err
			continue
		}
		plans, err := subscription.ListPlans(acc, token)
		if err != nil {
			lastErr = err
			continue
		}
		return map[string]interface{}{"success": true, "plans": plans}
	}
	msg := "全部账号均无法获取计划列表"
	if lastErr != nil {
		msg = lastErr.Error()
	}
	return map[string]interface{}{"success": false, "error": msg}
}

// GetSubscriptionLink 单账号获取支付/试用链接
func (a *App) GetSubscriptionLink(email, planType string) map[string]interface{} {
	items, err := data.LoadAccounts(storage.GetResultOutputDir())
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	var acc subscription.Account
	for _, m := range items {
		if e, _ := m["email"].(string); e == email {
			acc = accountFromMap(m)
			break
		}
	}
	if acc.Email == "" {
		return map[string]interface{}{"success": false, "error": "未找到账号: " + email}
	}
	token, err := subscription.RefreshAccessToken(acc)
	if err != nil {
		if subscription.IsSuspended(err) {
			removed, _ := data.DeleteAccount(storage.GetResultOutputDir(), email)
			subscription.DeleteCache(storage.GetDataDir(), email)
			log.Printf("[订阅] 账号 %s 已被封禁，已从输出文件移除 (removed=%v)", email, removed)
			return map[string]interface{}{"success": false, "error": err.Error(), "suspended": true, "removed": removed}
		}
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	url, err := subscription.CreateSubscriptionLink(acc, token, planType)
	if err != nil {
		if subscription.IsSuspended(err) {
			removed, _ := data.DeleteAccount(storage.GetResultOutputDir(), email)
			subscription.DeleteCache(storage.GetDataDir(), email)
			log.Printf("[订阅] 账号 %s 已被封禁，已从输出文件移除 (removed=%v)", email, removed)
			return map[string]interface{}{"success": false, "error": err.Error(), "suspended": true, "removed": removed}
		}
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	_ = subscription.PutCache(storage.GetDataDir(), email, url, planType)
	return map[string]interface{}{"success": true, "url": url}
}
