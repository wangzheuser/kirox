package core

import (
	"math/rand"
	"strings"
	"time"

	"reg_go/internal/email"
)

const (
	OutlookScopeIMAP  = "imap"
	OutlookScopeGraph = "graph"
)

// Config 注册配置
type Config struct {
	OIDCBase    string
	SigninBase  string
	ProfileBase string
	ViewBase    string
	PortalBase  string
	DirectoryID string
	StartURL    string

	KiroBase        string
	KiroRedirectURI string

	Password string
	FullName string

	PageStayMinMs int
	PageStayMaxMs int

	Proxy string
	// EmailProxy 表示邮箱服务 API 专用代理，空值表示直连。
	EmailProxy string
	Debug      bool
	// ProxyFromPool 表示 Proxy 是已从动态代理池中选出的运行时节点。
	ProxyFromPool bool
	// ProxySwitchable 表示当前代理背后可换节点，HTTP 传输错误应交给任务层切换节点。
	ProxySwitchable bool

	EmailProvider   string
	UseOutlook      bool
	OutlookAccount  *email.OutlookAccount
	OutlookScope    string
	OutlookOTPAfter time.Time

	UseMoeMail      bool
	MoeMailConfig   *email.MoeMailConfig
	MoeMailProvider *email.MoeMailProvider

	MoEmailBaseURL string
	MoEmailAPIKey  string
}

// NewConfig 创建默认配置
func NewConfig() *Config {
	return &Config{
		OIDCBase:        "https://oidc.us-east-1.amazonaws.com",
		SigninBase:      "https://us-east-1.signin.aws",
		ProfileBase:     "https://profile.aws.amazon.com",
		ViewBase:        "https://view.awsapps.com",
		PortalBase:      "https://portal.sso.us-east-1.amazonaws.com",
		DirectoryID:     "d-9067642ac7",
		StartURL:        "https://view.awsapps.com/start",
		KiroBase:        "https://app.kiro.dev",
		KiroRedirectURI: "https://app.kiro.dev/signin/oauth",
		Password:        GenPassword(),
		FullName:        "Test User",
		PageStayMinMs:   5000,
		PageStayMaxMs:   8000,
		OutlookScope:    OutlookScopeIMAP,
	}
}

// RandomPageStayMs 从配置区间内随机生成模拟页面停留时间。
func (c *Config) RandomPageStayMs() int {
	minMs := c.PageStayMinMs
	maxMs := c.PageStayMaxMs
	if minMs < 0 {
		minMs = 0
	}
	if maxMs < minMs {
		maxMs = minMs
	}
	if minMs == maxMs {
		return minMs
	}
	return minMs + rand.Intn(maxMs-minMs+1)
}

// UseOutlookGraph 判断当前 Outlook 账号是否使用 Microsoft Graph 读取邮件。
func (c *Config) UseOutlookGraph() bool {
	return strings.EqualFold(strings.TrimSpace(c.OutlookScope), OutlookScopeGraph)
}

// GenPassword 生成随机密码
func GenPassword() string {
	upper := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lower := "abcdefghijklmnopqrstuvwxyz"
	digits := "0123456789"
	special := "!@#$%^&*"

	var b strings.Builder
	for i := 0; i < 3; i++ {
		b.WriteByte(upper[rand.Intn(len(upper))])
	}
	for i := 0; i < 6; i++ {
		b.WriteByte(lower[rand.Intn(len(lower))])
	}
	for i := 0; i < 3; i++ {
		b.WriteByte(digits[rand.Intn(len(digits))])
	}
	for i := 0; i < 2; i++ {
		b.WriteByte(special[rand.Intn(len(special))])
	}
	pw := []byte(b.String())
	rand.Shuffle(len(pw), func(i, j int) { pw[i], pw[j] = pw[j], pw[i] })
	return string(pw)
}
