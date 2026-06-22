package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"reg_go/internal/proxy"
)

const (
	ProxyModeNone   = "none"
	ProxyModeNormal = "normal"
	ProxyModePool   = "pool"
	ProxyModeClash  = "clash"

	OutlookScopeIMAP  = "imap"
	OutlookScopeGraph = "graph"

	OutlookGraphRegistrationEmailAuto     = "auto"
	OutlookGraphRegistrationEmailImported = "imported"
	OutlookGraphRegistrationEmailPrimary  = "primary"

	DefaultRegistrationCount         = 1
	DefaultRegistrationSuccessTarget = 0
	DefaultRegistrationConcurrency   = 1
	DefaultRegistrationDelay         = 1

	RegistrationEmailProviderOutlook        = "outlook"
	RegistrationEmailProviderMoeMail        = "moemail"
	RegistrationEmailProviderMailporary     = "mailporary"
	RegistrationEmailProviderEmailnator     = "emailnator"
	RegistrationEmailProviderMailGW         = "mailgw"
	RegistrationEmailProviderMailTM         = "mailtm"
	RegistrationEmailProviderTempMailLOL    = "tempmail_lol"
	RegistrationEmailProviderGuerrilla      = "guerrillamail"
	RegistrationEmailProviderMailTemp       = "mailtemp"
	RegistrationEmailProviderTempMailPlus   = "tempmail_plus"
	RegistrationEmailProviderInboxKitten    = "inboxkitten"
	RegistrationEmailProviderInboxes        = "inboxes"
	RegistrationEmailProviderFreeCustom     = "freecustom"
	RegistrationEmailProviderDropMail       = "dropmail"
	RegistrationEmailProviderMailCatch      = "mailcatch"
	RegistrationEmailProviderTempMailo      = "tempmailo"
	RegistrationEmailProviderGeneratorEmail = "generator_email"
	RegistrationEmailProviderMailToWin      = "mailtowin"
	RegistrationEmailProviderMail2Me        = "mail2me"
	RegistrationEmailProviderPickMeMail     = "pickmemail"
	RegistrationEmailProviderMaxiMail       = "maximail"

	MoeMailDomainModeRandom = "random"
	MoeMailDomainModeAll    = "all"
	MoeMailDomainModeCustom = "custom"

	keyDataDir                      = "data_dir"
	keyResultOutputDir              = "result_output_dir"
	keyOutlookScope                 = "outlook_scope"
	keyOutlookGraphRegistrationMode = "outlook_graph_registration_email_mode"
	keyProxyMode                    = "proxy_mode"
	keyProxy                        = "proxy"
	keyLanguage                     = "language"
	keyClashProxy                   = "clash_proxy"
	keyEmailProxy                   = "email_proxy"
	keyKillSwitchEnabled            = "kill_switch_enabled"
	keySoundEnabled                 = "sound_enabled"
	keyVerifyModelsEnabled          = "verify_models_enabled"
	keyClashEnabled                 = "clash_enabled"
	keyClashAPIURL                  = "clash_api_url"
	keyClashAPISecret               = "clash_api_secret"
	keyClashProxyGroup              = "clash_proxy_group"
	keyClashTestURL                 = "clash_test_url"
	keyClashTestTimeout             = "clash_test_timeout"
	keyClashSkipConnectivityTest    = "clash_skip_connectivity_test"
	keyRegistrationConfigSaved      = "registration_config_saved"
	keyRegistrationCount            = "registration_count"
	keyRegistrationSuccessTarget    = "registration_success_target"
	keyRegistrationConcurrency      = "registration_concurrency"
	keyRegistrationDelay            = "registration_delay"
	keyRegistrationRetryCount       = "registration_retry_count"
	keyRegistrationOTPTimeout       = "registration_otp_timeout"
	keyRegistrationEmailProvider    = "registration_email_provider"
	keyRegistrationMoeMailMode      = "registration_moemail_domain_mode"
	keyRegistrationMoeMailDomains   = "registration_moemail_domains"
	keyRegistrationReuseFailedEmail = "registration_reuse_failed_email"
	keyKiroRSAPIURL                 = "kiro_rs_api_url"
	keyKiroRSAPIKey                 = "kiro_rs_api_key"
	keyKiroRSAutoSync               = "kiro_rs_auto_sync"
)

// RegistrationConfig 保存注册页面的业务配置。
type RegistrationConfig struct {
	Count             int      `json:"count"`
	SuccessTarget     int      `json:"successTarget"`
	Concurrency       int      `json:"concurrency"`
	Delay             int      `json:"delay"`
	RetryCount        int      `json:"retryCount"`
	OTPTimeout        int      `json:"otpTimeout"`
	EmailProvider     string   `json:"emailProvider"`
	MoeMailDomainMode string   `json:"moemailDomainMode"`
	MoeMailDomains    []string `json:"moemailDomains"`
	ReuseFailedEmail  bool     `json:"reuseFailedEmail"`
	Saved             bool     `json:"saved"`
}

var (
	configMu sync.Mutex

	_dataDir             string
	_dataDirOnce         sync.Once
	_resultOutputDir     string
	_resultOutputOnce    sync.Once
	_proxy               string
	_proxyOnce           sync.Once
	_killSwitchEnabled   bool
	_killSwitchOnce      sync.Once
	_soundEnabled        bool
	_soundOnce           sync.Once
	_verifyModelsEnabled bool
	_verifyModelsOnce    sync.Once
	_language            string
	_languageOnce        sync.Once
)

var configKeyOrder = []string{
	keyDataDir,
	keyResultOutputDir,
	keyOutlookScope,
	keyOutlookGraphRegistrationMode,
	keyProxyMode,
	keyProxy,
	keyLanguage,
	keyClashProxy,
	keyEmailProxy,
	keyKillSwitchEnabled,
	keySoundEnabled,
	keyVerifyModelsEnabled,
	keyClashEnabled,
	keyClashAPIURL,
	keyClashAPISecret,
	keyClashProxyGroup,
	keyClashTestURL,
	keyClashTestTimeout,
	keyClashSkipConnectivityTest,
	keyRegistrationConfigSaved,
	keyRegistrationCount,
	keyRegistrationSuccessTarget,
	keyRegistrationConcurrency,
	keyRegistrationDelay,
	keyRegistrationRetryCount,
	keyRegistrationOTPTimeout,
	keyRegistrationEmailProvider,
	keyRegistrationMoeMailMode,
	keyRegistrationMoeMailDomains,
	keyRegistrationReuseFailedEmail,
	keyKiroRSAPIURL,
	keyKiroRSAPIKey,
	keyKiroRSAutoSync,
}

var knownConfigKeys = func() map[string]bool {
	m := make(map[string]bool, len(configKeyOrder))
	for _, key := range configKeyOrder {
		m[key] = true
	}
	return m
}()

var deprecatedConfigKeys = map[string]bool{
	"outlook_register_domain" + "_override": true,
	"page_stay_min_ms":                      true,
	"page_stay_max_ms":                      true,
}

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

// readConfigFile 读取配置文件的底层函数，抽成变量便于测试注入读失败场景。
var readConfigFile = os.ReadFile

// loadConfigMap 解析 storage.conf 为 KV；兼容旧版（整文件即 data_dir 路径）。
// 读取/解析失败时返回默认空配置，供只读 getter 容错使用。
func loadConfigMap() map[string]string {
	configMu.Lock()
	defer configMu.Unlock()
	return loadConfigMapUnlocked()
}

func loadConfigMapUnlocked() map[string]string {
	m, _ := loadConfigMapStrict()
	return m
}

// loadConfigMapStrict 严格读取配置：
//   - 文件不存在（全新安装）→ 返回空 map, nil（合法初始状态）
//   - 文件存在但读取失败（被占用/权限等瞬时故障）→ 返回 nil 和错误，
//     供 modifyConfigMap 中止保存，避免用不完整数据覆盖磁盘上的完整配置。
func loadConfigMapStrict() (map[string]string, error) {
	m := map[string]string{}
	data, err := readConfigFile(getConfigFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return m, nil
	}
	if !strings.ContainsRune(text, '=') {
		m[keyDataDir] = text
		return m, nil
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
	return m, nil
}

func saveConfigMap(m map[string]string) error {
	configMu.Lock()
	defer configMu.Unlock()
	return saveConfigMapUnlocked(m)
}

func saveConfigMapUnlocked(m map[string]string) error {
	if err := os.MkdirAll(GetDefaultDataDir(), 0755); err != nil {
		return err
	}
	var b strings.Builder
	written := make(map[string]bool, len(m))
	writeKey := func(k string) {
		if deprecatedConfigKeys[k] {
			return
		}
		v := strings.TrimSpace(m[k])
		if v == "" {
			return
		}
		written[k] = true
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
		b.WriteByte('\n')
	}

	for _, k := range configKeyOrder {
		writeKey(k)
	}

	unknown := make([]string, 0)
	for k := range m {
		if knownConfigKeys[k] || deprecatedConfigKeys[k] || written[k] {
			continue
		}
		if strings.TrimSpace(m[k]) == "" {
			continue
		}
		unknown = append(unknown, k)
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		writeKey(k)
	}

	path := getConfigFilePath()

	// 写入前对现有配置做一次 .bak 备份，作为意外损坏时的恢复点（best-effort，失败不阻断保存）。
	if existing, err := readConfigFile(path); err == nil && len(existing) > 0 {
		_ = os.WriteFile(path+".bak", existing, 0600)
	}

	tmp := fmt.Sprintf("%s.tmp.%d.%d", path, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, []byte(b.String()), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func modifyConfigMap(fn func(map[string]string) error) error {
	configMu.Lock()
	defer configMu.Unlock()
	// 严格读取：若现有配置读取失败（被占用等瞬时故障），立即中止，
	// 绝不用不完整的内存数据覆盖磁盘，避免清空全部配置。
	m, err := loadConfigMapStrict()
	if err != nil {
		return fmt.Errorf("读取现有配置失败，已中止保存以防数据丢失: %w", err)
	}
	if err := fn(m); err != nil {
		return err
	}
	return saveConfigMapUnlocked(m)
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

	if err := modifyConfigMap(func(m map[string]string) error {
		m[keyDataDir] = dir
		return nil
	}); err != nil {
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

	_ = modifyConfigMap(func(m map[string]string) error {
		delete(m, keyDataDir)
		return nil
	})

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
	if err := modifyConfigMap(func(m map[string]string) error {
		m[keyResultOutputDir] = dir
		return nil
	}); err != nil {
		return "", fmt.Errorf("保存配置失败: %w", err)
	}
	_resultOutputDir = dir
	_resultOutputOnce = sync.Once{}
	_resultOutputOnce.Do(func() {})
	return dir, nil
}

// ResetResultOutputDir 重置为默认输出目录（用户文档目录下的 Kirox）
func ResetResultOutputDir() string {
	_ = modifyConfigMap(func(m map[string]string) error {
		delete(m, keyResultOutputDir)
		return nil
	})

	defaultDir := getDefaultResultOutputDir()
	os.MkdirAll(defaultDir, 0755)
	_resultOutputDir = defaultDir
	_resultOutputOnce = sync.Once{}
	_resultOutputOnce.Do(func() {})
	return defaultDir
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
	return modifyConfigMap(func(m map[string]string) error {
		m[keyOutlookScope] = normalized
		return nil
	})
}

// GetOutlookGraphRegistrationEmailMode 返回 Graph 模式下注册邮箱选择策略。
func GetOutlookGraphRegistrationEmailMode() string {
	m := loadConfigMap()
	mode := normalizeOutlookGraphRegistrationEmailMode(m[keyOutlookGraphRegistrationMode])
	if mode == "" {
		return OutlookGraphRegistrationEmailAuto
	}
	return mode
}

// SetOutlookGraphRegistrationEmailMode 保存 Graph 模式下注册邮箱选择策略。
func SetOutlookGraphRegistrationEmailMode(mode string) error {
	mode = normalizeOutlookGraphRegistrationEmailMode(mode)
	if mode == "" {
		return fmt.Errorf("不支持的 Outlook Graph 注册邮箱策略")
	}
	return modifyConfigMap(func(m map[string]string) error {
		m[keyOutlookGraphRegistrationMode] = mode
		return nil
	})
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
	if err := modifyConfigMap(func(m map[string]string) error {
		if normalized == "" {
			delete(m, keyProxy)
		} else {
			m[keyProxy] = normalized
		}
		return nil
	}); err != nil {
		return "", err
	}
	_proxy = normalized
	_proxyOnce = sync.Once{}
	_proxyOnce.Do(func() {})
	return normalized, nil
}

// ResetProxy 清空代理配置，恢复直连。
func ResetProxy() {
	_ = modifyConfigMap(func(m map[string]string) error {
		delete(m, keyProxy)
		return nil
	})
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
	if err := modifyConfigMap(func(m map[string]string) error {
		if normalized == "" {
			delete(m, keyEmailProxy)
		} else {
			m[keyEmailProxy] = normalized
		}
		return nil
	}); err != nil {
		return "", err
	}
	return normalized, nil
}

// ResetEmailProxy 清空邮箱 API 专用代理配置，恢复直连。
func ResetEmailProxy() {
	_ = modifyConfigMap(func(m map[string]string) error {
		delete(m, keyEmailProxy)
		return nil
	})
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
	return modifyConfigMap(func(m map[string]string) error {
		m[keyProxyMode] = mode
		return nil
	})
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
	if err := modifyConfigMap(func(m map[string]string) error {
		if normalized == "" {
			delete(m, keyClashProxy)
		} else {
			m[keyClashProxy] = normalized
		}
		return nil
	}); err != nil {
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
	return modifyConfigMap(func(m map[string]string) error {
		m[keyClashEnabled] = strconv.FormatBool(config.Enabled)
		m[keyClashAPIURL] = config.APIURL
		m[keyClashAPISecret] = config.APISecret
		m[keyClashProxyGroup] = config.ProxyGroup
		m[keyClashTestURL] = config.TestURL
		m[keyClashTestTimeout] = strconv.Itoa(config.TestTimeout)
		m[keyClashSkipConnectivityTest] = strconv.FormatBool(config.SkipConnectivityTest)
		return nil
	})
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

// GetSoundEnabled 返回任务结束提示音开关状态，默认开启。
func GetSoundEnabled() bool {
	_soundOnce.Do(func() {
		m := loadConfigMap()
		raw := strings.ToLower(strings.TrimSpace(m[keySoundEnabled]))
		_soundEnabled = raw != "false" && raw != "0" && raw != "no" && raw != "off"
	})
	return _soundEnabled
}

// SetSoundEnabled 保存任务结束提示音开关状态。
func SetSoundEnabled(enabled bool) error {
	if err := modifyConfigMap(func(m map[string]string) error {
		m[keySoundEnabled] = strconv.FormatBool(enabled)
		return nil
	}); err != nil {
		return err
	}
	_soundEnabled = enabled
	_soundOnce = sync.Once{}
	_soundOnce.Do(func() {})
	return nil
}

// GetVerifyModelsEnabled 返回注册验活时是否额外查询 ListAvailableModels，默认关闭。
func GetVerifyModelsEnabled() bool {
	_verifyModelsOnce.Do(func() {
		m := loadConfigMap()
		raw := strings.ToLower(strings.TrimSpace(m[keyVerifyModelsEnabled]))
		_verifyModelsEnabled = raw == "true" || raw == "1" || raw == "yes" || raw == "on"
	})
	return _verifyModelsEnabled
}

// SetVerifyModelsEnabled 保存注册验活二次模型检测开关状态。
func SetVerifyModelsEnabled(enabled bool) error {
	if err := modifyConfigMap(func(m map[string]string) error {
		m[keyVerifyModelsEnabled] = strconv.FormatBool(enabled)
		return nil
	}); err != nil {
		return err
	}
	_verifyModelsEnabled = enabled
	_verifyModelsOnce = sync.Once{}
	_verifyModelsOnce.Do(func() {})
	return nil
}

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

// GetRegistrationConfig 返回注册页业务配置。
func GetRegistrationConfig() RegistrationConfig {
	m := loadConfigMap()
	cfg := defaultRegistrationConfig()
	cfg.Saved = parseBool(m[keyRegistrationConfigSaved])

	if raw := strings.TrimSpace(m[keyRegistrationCount]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 1 {
			cfg.Count = value
			cfg.Saved = true
		}
	}
	if raw := strings.TrimSpace(m[keyRegistrationSuccessTarget]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			cfg.SuccessTarget = value
			cfg.Saved = true
		}
	}
	if raw := strings.TrimSpace(m[keyRegistrationConcurrency]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 1 {
			cfg.Concurrency = value
			cfg.Saved = true
		}
	}
	if raw := strings.TrimSpace(m[keyRegistrationDelay]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			cfg.Delay = value
			cfg.Saved = true
		}
	}
	if raw := strings.TrimSpace(m[keyRegistrationRetryCount]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 && value <= 5 {
			cfg.RetryCount = value
			cfg.Saved = true
		}
	}
	if raw := strings.TrimSpace(m[keyRegistrationOTPTimeout]); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 30 && value <= 600 {
			cfg.OTPTimeout = value
			cfg.Saved = true
		}
	}
	if raw := strings.TrimSpace(m[keyRegistrationEmailProvider]); raw != "" {
		if provider := normalizeRegistrationEmailProvider(raw); provider != "" {
			cfg.EmailProvider = provider
			cfg.Saved = true
		}
	}
	if raw := strings.TrimSpace(m[keyRegistrationMoeMailMode]); raw != "" {
		if mode := normalizeMoeMailDomainMode(raw); mode != "" {
			cfg.MoeMailDomainMode = mode
			cfg.Saved = true
		}
	}
	if domains := decodeStringList(m[keyRegistrationMoeMailDomains]); len(domains) > 0 {
		cfg.MoeMailDomains = domains
		cfg.Saved = true
		if cfg.MoeMailDomainMode == "" || cfg.MoeMailDomainMode == MoeMailDomainModeRandom {
			cfg.MoeMailDomainMode = MoeMailDomainModeCustom
		}
	}
	if raw := strings.TrimSpace(m[keyRegistrationReuseFailedEmail]); raw != "" {
		cfg.ReuseFailedEmail = parseBool(raw)
		cfg.Saved = true
	}

	normalized, err := normalizeRegistrationConfig(cfg)
	if err != nil {
		return defaultRegistrationConfig()
	}
	normalized.Saved = cfg.Saved
	return normalized
}

// SetRegistrationConfig 保存注册页业务配置。
func SetRegistrationConfig(cfg RegistrationConfig) error {
	normalized, err := normalizeRegistrationConfig(cfg)
	if err != nil {
		return err
	}
	return modifyConfigMap(func(m map[string]string) error {
		m[keyRegistrationConfigSaved] = "true"
		m[keyRegistrationCount] = strconv.Itoa(normalized.Count)
		m[keyRegistrationSuccessTarget] = strconv.Itoa(normalized.SuccessTarget)
		m[keyRegistrationConcurrency] = strconv.Itoa(normalized.Concurrency)
		m[keyRegistrationDelay] = strconv.Itoa(normalized.Delay)
		m[keyRegistrationRetryCount] = strconv.Itoa(normalized.RetryCount)
		m[keyRegistrationOTPTimeout] = strconv.Itoa(normalized.OTPTimeout)
		m[keyRegistrationEmailProvider] = normalized.EmailProvider
		m[keyRegistrationMoeMailMode] = normalized.MoeMailDomainMode
		m[keyRegistrationReuseFailedEmail] = strconv.FormatBool(normalized.ReuseFailedEmail)
		if len(normalized.MoeMailDomains) == 0 {
			delete(m, keyRegistrationMoeMailDomains)
		} else {
			m[keyRegistrationMoeMailDomains] = encodeStringList(normalized.MoeMailDomains)
		}
		return nil
	})
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
	case ProxyModePool:
		return ProxyModePool
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

func normalizeOutlookGraphRegistrationEmailMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case OutlookGraphRegistrationEmailAuto, "":
		return OutlookGraphRegistrationEmailAuto
	case OutlookGraphRegistrationEmailImported:
		return OutlookGraphRegistrationEmailImported
	case OutlookGraphRegistrationEmailPrimary:
		return OutlookGraphRegistrationEmailPrimary
	default:
		return ""
	}
}

func defaultRegistrationConfig() RegistrationConfig {
	return RegistrationConfig{
		Count:             DefaultRegistrationCount,
		SuccessTarget:     DefaultRegistrationSuccessTarget,
		Concurrency:       DefaultRegistrationConcurrency,
		Delay:             DefaultRegistrationDelay,
		RetryCount:        1,
		OTPTimeout:        60,
		EmailProvider:     RegistrationEmailProviderOutlook,
		MoeMailDomainMode: MoeMailDomainModeRandom,
		MoeMailDomains:    []string{},
		Saved:             false,
	}
}

func normalizeRegistrationConfig(cfg RegistrationConfig) (RegistrationConfig, error) {
	out := cfg
	if out.Count < 1 {
		return RegistrationConfig{}, fmt.Errorf("注册数量必须大于或等于 1")
	}
	if out.SuccessTarget < 0 {
		return RegistrationConfig{}, fmt.Errorf("注册成功数量不能小于 0")
	}
	if out.Concurrency < 1 {
		return RegistrationConfig{}, fmt.Errorf("并发数必须大于或等于 1")
	}
	if out.Delay < 0 {
		return RegistrationConfig{}, fmt.Errorf("任务间隔不能小于 0")
	}
	if out.RetryCount < 0 || out.RetryCount > 5 {
		return RegistrationConfig{}, fmt.Errorf("重试次数必须在 0-5 之间")
	}
	if out.OTPTimeout < 30 {
		out.OTPTimeout = 30
	}
	if out.OTPTimeout > 600 {
		out.OTPTimeout = 600
	}

	provider := normalizeRegistrationEmailProvider(out.EmailProvider)
	if provider == "" {
		return RegistrationConfig{}, fmt.Errorf("未知邮箱提供商")
	}
	out.EmailProvider = provider

	domains, sentinelMode := normalizeMoeMailDomains(out.MoeMailDomains)
	mode := normalizeMoeMailDomainMode(out.MoeMailDomainMode)
	if sentinelMode != "" {
		mode = sentinelMode
	}
	if mode == "" {
		if len(domains) > 0 {
			mode = MoeMailDomainModeCustom
		} else {
			mode = MoeMailDomainModeRandom
		}
	}
	out.MoeMailDomainMode = mode
	if mode == MoeMailDomainModeCustom {
		out.MoeMailDomains = domains
	} else {
		out.MoeMailDomains = []string{}
	}
	return out, nil
}

func normalizeRegistrationEmailProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", RegistrationEmailProviderOutlook:
		return RegistrationEmailProviderOutlook
	case RegistrationEmailProviderMoeMail:
		return RegistrationEmailProviderMoeMail
	case RegistrationEmailProviderMailporary:
		return RegistrationEmailProviderMailporary
	case RegistrationEmailProviderEmailnator:
		return RegistrationEmailProviderEmailnator
	case RegistrationEmailProviderMailGW:
		return RegistrationEmailProviderMailGW
	case RegistrationEmailProviderMailTM:
		return RegistrationEmailProviderMailTM
	case RegistrationEmailProviderTempMailLOL:
		return RegistrationEmailProviderTempMailLOL
	case RegistrationEmailProviderGuerrilla:
		return RegistrationEmailProviderGuerrilla
	case RegistrationEmailProviderMailTemp:
		return RegistrationEmailProviderMailTemp
	case RegistrationEmailProviderTempMailPlus:
		return RegistrationEmailProviderTempMailPlus
	case RegistrationEmailProviderInboxKitten:
		return RegistrationEmailProviderInboxKitten
	case RegistrationEmailProviderInboxes:
		return RegistrationEmailProviderInboxes
	case RegistrationEmailProviderFreeCustom:
		return RegistrationEmailProviderFreeCustom
	case RegistrationEmailProviderDropMail:
		return RegistrationEmailProviderDropMail
	case RegistrationEmailProviderMailCatch:
		return RegistrationEmailProviderMailCatch
	case RegistrationEmailProviderTempMailo:
		return RegistrationEmailProviderTempMailo
	case RegistrationEmailProviderGeneratorEmail:
		return RegistrationEmailProviderGeneratorEmail
	case RegistrationEmailProviderMailToWin:
		return RegistrationEmailProviderMailToWin
	case RegistrationEmailProviderMail2Me:
		return RegistrationEmailProviderMail2Me
	case RegistrationEmailProviderPickMeMail:
		return RegistrationEmailProviderPickMeMail
	case RegistrationEmailProviderMaxiMail:
		return RegistrationEmailProviderMaxiMail
	default:
		return ""
	}
}

func normalizeMoeMailDomainMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", MoeMailDomainModeRandom:
		return MoeMailDomainModeRandom
	case MoeMailDomainModeAll:
		return MoeMailDomainModeAll
	case MoeMailDomainModeCustom:
		return MoeMailDomainModeCustom
	default:
		return ""
	}
}

func normalizeMoeMailDomains(raw []string) ([]string, string) {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	sentinelMode := ""
	for _, item := range raw {
		domain := strings.ToLower(strings.TrimSpace(item))
		switch domain {
		case "", "__random__":
			if domain == "__random__" {
				sentinelMode = MoeMailDomainModeRandom
			}
			continue
		case "__all__":
			sentinelMode = MoeMailDomainModeAll
			continue
		}
		if seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	return out, sentinelMode
}

func encodeStringList(items []string) string {
	data, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(data)
}

func decodeStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	out, _ := normalizeMoeMailDomains(items)
	return out
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
	if err := modifyConfigMap(func(m map[string]string) error {
		if enabled {
			m[keyKillSwitchEnabled] = "true"
		} else {
			m[keyKillSwitchEnabled] = "false"
		}
		return nil
	}); err != nil {
		return err
	}
	_killSwitchEnabled = enabled
	_killSwitchOnce = sync.Once{}
	_killSwitchOnce.Do(func() {})
	return nil
}

// GetLanguage 返回当前界面语言代码（"zh"/"en"/"ja"），未设置时返回空字符串。
func GetLanguage() string {
	_languageOnce.Do(func() {
		m := loadConfigMap()
		_language = strings.TrimSpace(m[keyLanguage])
	})
	return _language
}

// SetLanguage 持久化界面语言；仅接受 "zh"/"en"/"ja"，其他值返回错误。
func SetLanguage(lang string) error {
	lang = strings.TrimSpace(lang)
	if lang != "zh" && lang != "en" && lang != "ja" {
		return fmt.Errorf("不支持的语言: %s", lang)
	}
	m := loadConfigMap()
	m[keyLanguage] = lang
	if err := saveConfigMap(m); err != nil {
		return err
	}
	_language = lang
	_languageOnce = sync.Once{}
	_languageOnce.Do(func() {})
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
