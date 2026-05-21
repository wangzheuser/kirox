package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"reg_go/internal/proxy"
)

const (
	ProxyModeNone   = "none"
	ProxyModeNormal = "normal"
	ProxyModeClash  = "clash"

	OutlookScopeIMAP  = "imap"
	OutlookScopeGraph = "graph"

	DefaultPageStayMinMs = 5000
	DefaultPageStayMaxMs = 8000

	keyDataDir                   = "data_dir"
	keyResultOutputDir           = "result_output_dir"
	keyPageStayMinMs             = "page_stay_min_ms"
	keyPageStayMaxMs             = "page_stay_max_ms"
	keyOutlookScope              = "outlook_scope"
	keyOutlookRegisterDomain     = "outlook_register_domain_override"
	keyProxyMode                 = "proxy_mode"
	keyProxy                     = "proxy"
	keyClashProxy                = "clash_proxy"
	keyEmailProxy                = "email_proxy"
	keyKillSwitchEnabled         = "kill_switch_enabled"
	keyClashEnabled              = "clash_enabled"
	keyClashAPIURL               = "clash_api_url"
	keyClashAPISecret            = "clash_api_secret"
	keyClashProxyGroup           = "clash_proxy_group"
	keyClashTestURL              = "clash_test_url"
	keyClashTestTimeout          = "clash_test_timeout"
	keyClashSkipConnectivityTest = "clash_skip_connectivity_test"
)

// PageStayConfig 保存发送验证码前模拟页面停留的随机区间。
type PageStayConfig struct {
	MinMs int `json:"minMs"`
	MaxMs int `json:"maxMs"`
}

var (
	_dataDir           string
	_dataDirOnce       sync.Once
	_resultOutputDir   string
	_resultOutputOnce  sync.Once
	_proxy             string
	_proxyOnce         sync.Once
	_killSwitchEnabled bool
	_killSwitchOnce    sync.Once
)

// GetDefaultDataDir 获取默认应用数据目录
func GetDefaultDataDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	return filepath.Join(configDir, "kirox")
}

// getConfigFilePath 获取配置文件路径（始终在默认目录下）
func getConfigFilePath() string {
	return filepath.Join(GetDefaultDataDir(), "storage.conf")
}

// loadConfigMap 解析 storage.conf 为 KV；兼容旧版（整文件即 data_dir 路径）
func loadConfigMap() map[string]string {
	m := map[string]string{}
	data, err := os.ReadFile(getConfigFilePath())
	if err != nil {
		return m
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return m
	}
	if !strings.ContainsRune(text, '=') {
		m[keyDataDir] = text
		return m
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		if k != "" {
			m[k] = v
		}
	}
	return m
}

func saveConfigMap(m map[string]string) error {
	os.MkdirAll(GetDefaultDataDir(), 0755)
	var b strings.Builder
	for _, k := range []string{
		keyDataDir,
		keyResultOutputDir,
		keyPageStayMinMs,
		keyPageStayMaxMs,
		keyOutlookScope,
		keyOutlookRegisterDomain,
		keyProxyMode,
		keyProxy,
		keyClashProxy,
		keyEmailProxy,
		keyKillSwitchEnabled,
		keyClashEnabled,
		keyClashAPIURL,
		keyClashAPISecret,
		keyClashProxyGroup,
		keyClashTestURL,
		keyClashTestTimeout,
		keyClashSkipConnectivityTest,
	} {
		if v := strings.TrimSpace(m[k]); v != "" {
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	return os.WriteFile(getConfigFilePath(), []byte(b.String()), 0600)
}

// GetDataDir 获取应用数据目录（优先使用自定义目录）
func GetDataDir() string {
	_dataDirOnce.Do(func() {
		m := loadConfigMap()
		custom := strings.TrimSpace(m[keyDataDir])
		if custom != "" {
			if info, err := os.Stat(custom); err == nil && info.IsDir() {
				_dataDir = custom
			}
		}
		if _dataDir == "" {
			_dataDir = GetDefaultDataDir()
		}
		os.MkdirAll(_dataDir, 0755)
	})
	return _dataDir
}

// SetDataDirPath 设置自定义存储目录（自动迁移 accounts.dat）
func SetDataDirPath(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("目录不能为空")
	}
	oldDir := GetDataDir()

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	if oldDir != "" && oldDir != dir {
		migrated, migErr := migrateData(oldDir, dir)
		if migErr != nil {
			return "", fmt.Errorf("数据迁移失败: %w", migErr)
		}
		if migrated > 0 {
			log.Printf("已迁移 %d 个数据文件: %s → %s", migrated, oldDir, dir)
		}
	}

	m := loadConfigMap()
	m[keyDataDir] = dir
	if err := saveConfigMap(m); err != nil {
		return "", fmt.Errorf("保存配置失败: %w", err)
	}

	_dataDir = dir
	_dataDirOnce = sync.Once{}
	_dataDirOnce.Do(func() {})

	return dir, nil
}

// ResetDataDirPath 重置为默认存储目录（自动迁移数据回默认目录）
func ResetDataDirPath() string {
	oldDir := GetDataDir()
	defaultDir := GetDefaultDataDir()

	if oldDir != "" && oldDir != defaultDir {
		migrated, _ := migrateData(oldDir, defaultDir)
		if migrated > 0 {
			log.Printf("已迁移 %d 个数据文件: %s → %s", migrated, oldDir, defaultDir)
		}
	}

	m := loadConfigMap()
	delete(m, keyDataDir)
	_ = saveConfigMap(m)

	os.MkdirAll(defaultDir, 0755)
	_dataDir = defaultDir
	_dataDirOnce = sync.Once{}
	_dataDirOnce.Do(func() {})

	return defaultDir
}

// getDefaultResultOutputDir 默认输出目录：用户文档目录下的 Kirox 文件夹。
// 若无法解析用户主目录，回落到可执行文件所在目录下的 output。
func getDefaultResultOutputDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Documents", "Kirox")
	}
	base := "."
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		base = filepath.Dir(exe)
	} else if cwd, err := os.Getwd(); err == nil {
		base = cwd
	}
	return filepath.Join(base, "output")
}

// GetResultOutputDir 获取注册结果输出目录（默认为用户文档目录下的 Kirox）
func GetResultOutputDir() string {
	_resultOutputOnce.Do(func() {
		m := loadConfigMap()
		if custom := strings.TrimSpace(m[keyResultOutputDir]); custom != "" {
			_resultOutputDir = custom
		} else {
			_resultOutputDir = getDefaultResultOutputDir()
		}
		os.MkdirAll(_resultOutputDir, 0755)
	})
	return _resultOutputDir
}

// SetResultOutputDir 设置自定义输出目录（不迁移已有 JSON 文件）
func SetResultOutputDir(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("目录不能为空")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	m := loadConfigMap()
	m[keyResultOutputDir] = dir
	if err := saveConfigMap(m); err != nil {
		return "", fmt.Errorf("保存配置失败: %w", err)
	}
	_resultOutputDir = dir
	_resultOutputOnce = sync.Once{}
	_resultOutputOnce.Do(func() {})
	return dir, nil
}

// ResetResultOutputDir 重置为默认输出目录（用户文档目录下的 Kirox）
func ResetResultOutputDir() string {
	m := loadConfigMap()
	delete(m, keyResultOutputDir)
	_ = saveConfigMap(m)

	defaultDir := getDefaultResultOutputDir()
	os.MkdirAll(defaultDir, 0755)
	_resultOutputDir = defaultDir
	_resultOutputOnce = sync.Once{}
	_resultOutputOnce.Do(func() {})
	return defaultDir
}

// GetPageStayConfig 获取模拟页面停留时间配置，默认保持 5-8 秒随机。
func GetPageStayConfig() PageStayConfig {
	m := loadConfigMap()
	cfg := PageStayConfig{
		MinMs: DefaultPageStayMinMs,
		MaxMs: DefaultPageStayMaxMs,
	}
	if raw := strings.TrimSpace(m[keyPageStayMinMs]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			cfg.MinMs = value
		}
	}
	if raw := strings.TrimSpace(m[keyPageStayMaxMs]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			cfg.MaxMs = value
		}
	}
	if err := ValidatePageStayConfig(cfg); err != nil {
		return PageStayConfig{MinMs: DefaultPageStayMinMs, MaxMs: DefaultPageStayMaxMs}
	}
	return cfg
}

// SetPageStayConfig 保存模拟页面停留时间配置。
func SetPageStayConfig(cfg PageStayConfig) error {
	if err := ValidatePageStayConfig(cfg); err != nil {
		return err
	}
	m := loadConfigMap()
	m[keyPageStayMinMs] = strconv.Itoa(cfg.MinMs)
	m[keyPageStayMaxMs] = strconv.Itoa(cfg.MaxMs)
	return saveConfigMap(m)
}

// ValidatePageStayConfig 校验模拟页面停留时间配置。
func ValidatePageStayConfig(cfg PageStayConfig) error {
	if cfg.MinMs < 0 || cfg.MaxMs < 0 {
		return fmt.Errorf("页面停留时间不能小于 0")
	}
	if cfg.MinMs > cfg.MaxMs {
		return fmt.Errorf("页面停留最小值不能大于最大值")
	}
	return nil
}

// GetOutlookScope 返回 Outlook 验证码读取方式，默认使用 IMAP 保持兼容。
func GetOutlookScope() string {
	m := loadConfigMap()
	if scope := normalizeOutlookScope(m[keyOutlookScope]); scope != "" {
		return scope
	}
	return OutlookScopeIMAP
}

// SetOutlookScope 设置 Outlook 验证码读取方式。
func SetOutlookScope(scope string) error {
	if strings.TrimSpace(scope) == "" {
		return fmt.Errorf("未知 Outlook 读取方式")
	}
	normalized := normalizeOutlookScope(scope)
	if normalized == "" {
		return fmt.Errorf("未知 Outlook 读取方式")
	}
	m := loadConfigMap()
	m[keyOutlookScope] = normalized
	return saveConfigMap(m)
}

// GetOutlookRegisterDomainOverride 返回 Outlook 注册邮箱后缀覆盖配置，空字符串表示关闭。
func GetOutlookRegisterDomainOverride() string {
	m := loadConfigMap()
	domain, err := NormalizeOutlookRegisterDomainOverride(m[keyOutlookRegisterDomain])
	if err != nil {
		return ""
	}
	return domain
}

// SetOutlookRegisterDomainOverride 保存 Outlook 注册邮箱后缀覆盖配置。
func SetOutlookRegisterDomainOverride(raw string) (string, error) {
	domain, err := NormalizeOutlookRegisterDomainOverride(raw)
	if err != nil {
		return "", err
	}
	m := loadConfigMap()
	if domain == "" {
		delete(m, keyOutlookRegisterDomain)
	} else {
		m[keyOutlookRegisterDomain] = domain
	}
	return domain, saveConfigMap(m)
}

// NormalizeOutlookRegisterDomainOverride 规范化并校验 Outlook 注册邮箱后缀覆盖域名。
func NormalizeOutlookRegisterDomainOverride(raw string) (string, error) {
	domain := strings.ToLower(strings.TrimSpace(raw))
	domain = strings.TrimPrefix(domain, "@")
	if domain == "" {
		return "", nil
	}
	if strings.Contains(domain, "://") || strings.ContainsAny(domain, "/\\?#@:") {
		return "", fmt.Errorf("Outlook 注册邮箱后缀只能填写域名，例如 outlook.fr")
	}
	if strings.IndexFunc(domain, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf("Outlook 注册邮箱后缀不能包含空白字符")
	}
	if len(domain) > 253 {
		return "", fmt.Errorf("Outlook 注册邮箱后缀过长")
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("Outlook 注册邮箱后缀必须是完整域名，例如 outlook.fr")
	}
	for _, label := range labels {
		if !validDomainLabel(label) {
			return "", fmt.Errorf("Outlook 注册邮箱后缀格式无效")
		}
	}
	return domain, nil
}

// GetProxy 返回当前全局代理 URL（空字符串表示直连）。
func GetProxy() string {
	_proxyOnce.Do(func() {
		m := loadConfigMap()
		_proxy = strings.TrimSpace(m[keyProxy])
	})
	return _proxy
}

// SetProxy 设置全局代理 URL（会自动归一化常见简写格式）。
func SetProxy(raw string) (string, error) {
	normalized := NormalizeProxyAddress(strings.TrimSpace(raw))
	m := loadConfigMap()
	if normalized == "" {
		delete(m, keyProxy)
	} else {
		m[keyProxy] = normalized
	}
	if err := saveConfigMap(m); err != nil {
		return "", err
	}
	_proxy = normalized
	_proxyOnce = sync.Once{}
	_proxyOnce.Do(func() {})
	return normalized, nil
}

// ResetProxy 清空代理配置，恢复直连。
func ResetProxy() {
	m := loadConfigMap()
	delete(m, keyProxy)
	_ = saveConfigMap(m)
	_proxy = ""
	_proxyOnce = sync.Once{}
	_proxyOnce.Do(func() {})
}

// GetEmailProxy 返回邮箱 API 专用代理 URL（空字符串表示直连）。
func GetEmailProxy() string {
	m := loadConfigMap()
	return strings.TrimSpace(m[keyEmailProxy])
}

// SetEmailProxy 设置邮箱 API 专用代理 URL（会自动归一化常见简写格式）。
func SetEmailProxy(raw string) (string, error) {
	normalized := NormalizeProxyAddress(strings.TrimSpace(raw))
	m := loadConfigMap()
	if normalized == "" {
		delete(m, keyEmailProxy)
	} else {
		m[keyEmailProxy] = normalized
	}
	if err := saveConfigMap(m); err != nil {
		return "", err
	}
	return normalized, nil
}

// ResetEmailProxy 清空邮箱 API 专用代理配置，恢复直连。
func ResetEmailProxy() {
	m := loadConfigMap()
	delete(m, keyEmailProxy)
	_ = saveConfigMap(m)
}

// GetProxyMode 返回当前互斥代理模式。
func GetProxyMode() string {
	m := loadConfigMap()
	if mode := normalizeProxyMode(m[keyProxyMode]); mode != "" {
		return mode
	}
	if parseBool(m[keyClashEnabled]) && looksLikeLocalProxy(m[keyProxy]) {
		return ProxyModeClash
	}
	if strings.TrimSpace(m[keyProxy]) != "" {
		return ProxyModeNormal
	}
	return ProxyModeNone
}

// SetProxyMode 设置当前互斥代理模式，不清空其他模式配置。
func SetProxyMode(mode string) error {
	mode = normalizeProxyMode(mode)
	if mode == "" {
		return fmt.Errorf("未知代理模式")
	}
	m := loadConfigMap()
	m[keyProxyMode] = mode
	return saveConfigMap(m)
}

// GetClashProxy 返回 Clash 本地代理地址。
func GetClashProxy() string {
	m := loadConfigMap()
	if value := strings.TrimSpace(m[keyClashProxy]); value != "" {
		return value
	}
	if parseBool(m[keyClashEnabled]) && looksLikeLocalProxy(m[keyProxy]) {
		return strings.TrimSpace(m[keyProxy])
	}
	return ""
}

// SetClashProxy 设置 Clash 本地代理地址。
func SetClashProxy(raw string) (string, error) {
	normalized := NormalizeProxyAddress(strings.TrimSpace(raw))
	m := loadConfigMap()
	if normalized == "" {
		delete(m, keyClashProxy)
	} else {
		m[keyClashProxy] = normalized
	}
	if err := saveConfigMap(m); err != nil {
		return "", err
	}
	return normalized, nil
}

// GetClashConfig 返回 Clash API 自动切换配置。
func GetClashConfig() proxy.ClashConfig {
	m := loadConfigMap()
	timeout, _ := strconv.Atoi(strings.TrimSpace(m[keyClashTestTimeout]))
	return proxy.NormalizeClashConfig(proxy.ClashConfig{
		Enabled:              parseBool(m[keyClashEnabled]),
		APIURL:               strings.TrimSpace(m[keyClashAPIURL]),
		APISecret:            strings.TrimSpace(m[keyClashAPISecret]),
		ProxyGroup:           strings.TrimSpace(m[keyClashProxyGroup]),
		TestURL:              strings.TrimSpace(m[keyClashTestURL]),
		TestTimeout:          timeout,
		SkipConnectivityTest: parseBool(m[keyClashSkipConnectivityTest]),
	})
}

// SetClashConfig 保存 Clash API 自动切换配置。
func SetClashConfig(config proxy.ClashConfig) error {
	config = proxy.NormalizeClashConfig(config)
	m := loadConfigMap()
	m[keyClashEnabled] = strconv.FormatBool(config.Enabled)
	m[keyClashAPIURL] = config.APIURL
	m[keyClashAPISecret] = config.APISecret
	m[keyClashProxyGroup] = config.ProxyGroup
	m[keyClashTestURL] = config.TestURL
	m[keyClashTestTimeout] = strconv.Itoa(config.TestTimeout)
	m[keyClashSkipConnectivityTest] = strconv.FormatBool(config.SkipConnectivityTest)
	return saveConfigMap(m)
}

// GetKillSwitchEnabled 返回熔断级错误自动停止开关状态，默认开启。
func GetKillSwitchEnabled() bool {
	_killSwitchOnce.Do(func() {
		m := loadConfigMap()
		raw := strings.ToLower(strings.TrimSpace(m[keyKillSwitchEnabled]))
		_killSwitchEnabled = raw != "false" && raw != "0" && raw != "no" && raw != "off"
	})
	return _killSwitchEnabled
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeProxyMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ProxyModeNone, "":
		if strings.TrimSpace(mode) == "" {
			return ""
		}
		return ProxyModeNone
	case ProxyModeNormal:
		return ProxyModeNormal
	case ProxyModeClash:
		return ProxyModeClash
	default:
		return ""
	}
}

func normalizeOutlookScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case OutlookScopeIMAP, "":
		return OutlookScopeIMAP
	case OutlookScopeGraph:
		return OutlookScopeGraph
	default:
		return ""
	}
}

func validDomainLabel(label string) bool {
	if label == "" || len(label) > 63 {
		return false
	}
	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return false
	}
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}

func looksLikeLocalProxy(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	normalized := NormalizeProxyAddress(raw)
	hostPort := normalized
	if i := strings.Index(normalized, "://"); i >= 0 {
		hostPort = normalized[i+3:]
		if j := strings.IndexByte(hostPort, '/'); j >= 0 {
			hostPort = hostPort[:j]
		}
		if j := strings.LastIndexByte(hostPort, '@'); j >= 0 {
			hostPort = hostPort[j+1:]
		}
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || host == "::1" || strings.HasPrefix(host, "127.") || host == "0.0.0.0"
}

// SetKillSwitchEnabled 设置熔断级错误自动停止开关状态。
func SetKillSwitchEnabled(enabled bool) error {
	m := loadConfigMap()
	if enabled {
		m[keyKillSwitchEnabled] = "true"
	} else {
		m[keyKillSwitchEnabled] = "false"
	}
	if err := saveConfigMap(m); err != nil {
		return err
	}
	_killSwitchEnabled = enabled
	_killSwitchOnce = sync.Once{}
	_killSwitchOnce.Do(func() {})
	return nil
}

// NormalizeProxyAddress 归一化常见代理写法为完整 URL:
//   - 已带 scheme 的 URL 原样返回
//   - host:port:user:pass -> http://user:pass@host:port (cliproxy 等导出格式)
//   - host:port -> socks5://host:port
//   - user:pass@host:port -> http://user:pass@host:port
func NormalizeProxyAddress(s string) string {
	if s == "" {
		return ""
	}
	if strings.Contains(s, "://") {
		return s
	}
	if strings.Contains(s, "@") {
		return "http://" + s
	}
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 4:
		host, port, user, pass := parts[0], parts[1], parts[2], parts[3]
		if host != "" && port != "" {
			return fmt.Sprintf("http://%s:%s@%s:%s", user, pass, host, port)
		}
	case 2:
		return "socks5://" + s
	}
	return s
}

// migrateData 将旧目录中的数据文件迁移到新目录
func migrateData(oldDir, newDir string) (int, error) {
	migrated := 0
	items := []string{"accounts.json", "accounts.dat"}

	for _, item := range items {
		src := filepath.Join(oldDir, item)
		dst := filepath.Join(newDir, "accounts.json")

		if _, err := os.Stat(src); err != nil {
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return migrated, err
		}
		os.MkdirAll(filepath.Dir(dst), 0755)
		if err := os.WriteFile(dst, data, 0600); err != nil {
			return migrated, err
		}
		migrated++
	}
	return migrated, nil
}

// GetAccountsPath 获取微软邮箱账号文件路径
func GetAccountsPath() string {
	return filepath.Join(GetDataDir(), "accounts.json")
}

// ===== Accounts 内存缓存（消除并发文件 I/O 瓶颈）=====

var (
	_accountsCache  []map[string]interface{}
	_accountsMu     sync.RWMutex
	_accountsLoaded bool
	_accountsDirty  bool
	_flushTimer     *time.Timer
)

func loadAccountsCache() {
	if _accountsLoaded {
		return
	}
	data, err := loadJSON(GetAccountsPath())
	if err != nil {
		_accountsCache = []map[string]interface{}{}
	} else {
		_accountsCache = data
	}
	_accountsLoaded = true
}

// GetAccountsCached 获取账号列表（从内存缓存）
func GetAccountsCached() []map[string]interface{} {
	_accountsMu.Lock()
	if !_accountsLoaded {
		loadAccountsCache()
	}
	result := make([]map[string]interface{}, len(_accountsCache))
	copy(result, _accountsCache)
	_accountsMu.Unlock()
	return result
}

// SetAccountsCached 替换账号列表并触发异步刷盘
func SetAccountsCached(accounts []map[string]interface{}) {
	_accountsMu.Lock()
	_accountsCache = accounts
	_accountsLoaded = true
	_accountsDirty = true
	scheduleFlush()
	_accountsMu.Unlock()
}

// ModifyAccountsCached 原子修改账号列表（回调在锁内执行，高效无文件 I/O）
func ModifyAccountsCached(fn func([]map[string]interface{}) []map[string]interface{}) {
	_accountsMu.Lock()
	if !_accountsLoaded {
		loadAccountsCache()
	}
	_accountsCache = fn(_accountsCache)
	_accountsDirty = true
	scheduleFlush()
	_accountsMu.Unlock()
}

func scheduleFlush() {
	if _flushTimer != nil {
		_flushTimer.Stop()
	}
	_flushTimer = time.AfterFunc(500*time.Millisecond, flushAccountsToDisk)
}

func flushAccountsToDisk() {
	_accountsMu.RLock()
	if !_accountsDirty {
		_accountsMu.RUnlock()
		return
	}
	data := make([]map[string]interface{}, len(_accountsCache))
	copy(data, _accountsCache)
	_accountsMu.RUnlock()

	err := SaveJSON(GetAccountsPath(), data)

	_accountsMu.Lock()
	if err == nil {
		_accountsDirty = false
	}
	_accountsMu.Unlock()
}

// FlushAccountsSync 同步刷盘（程序退出前调用）
func FlushAccountsSync() {
	if _flushTimer != nil {
		_flushTimer.Stop()
	}
	flushAccountsToDisk()
}

// ===== JSON 存储读写 =====

var fileMutexes sync.Map

func getFileMutex(filePath string) *sync.Mutex {
	val, _ := fileMutexes.LoadOrStore(filePath, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// LoadJSON 从文件读取 JSON 数组（线程安全）
func LoadJSON(filePath string) ([]map[string]interface{}, error) {
	mu := getFileMutex(filePath)
	mu.Lock()
	defer mu.Unlock()
	return loadJSON(filePath)
}

// SaveJSON 将 JSON 数组写入文件（线程安全，原子写入）
func SaveJSON(filePath string, items []map[string]interface{}) error {
	mu := getFileMutex(filePath)
	mu.Lock()
	defer mu.Unlock()
	return saveJSON(filePath, items)
}

// AppendJSON 向 JSON 数组文件追加一条记录（线程安全）
func AppendJSON(filePath string, item map[string]interface{}) error {
	mu := getFileMutex(filePath)
	mu.Lock()
	defer mu.Unlock()
	existing, _ := loadJSON(filePath)
	existing = append(existing, item)
	return saveJSON(filePath, existing)
}

// ModifyJSON 原子读-改-写
func ModifyJSON(filePath string, fn func([]map[string]interface{}) []map[string]interface{}) error {
	mu := getFileMutex(filePath)
	mu.Lock()
	defer mu.Unlock()
	existing, _ := loadJSON(filePath)
	return saveJSON(filePath, fn(existing))
}

// CountJSON 统计 JSON 数组文件中的记录数
func CountJSON(filePath string) int {
	items, err := LoadJSON(filePath)
	if err != nil {
		return 0
	}
	return len(items)
}

func loadJSON(filePath string) ([]map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func saveJSON(filePath string, items []map[string]interface{}) error {
	b, err := json.Marshal(items)
	if err != nil {
		return err
	}
	os.MkdirAll(filepath.Dir(filePath), 0755)
	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}
