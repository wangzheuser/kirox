package email

import (
	"log"
)

// TempEmailService 临时邮箱服务接口
type TempEmailService interface {
	// Create 创建临时邮箱，返回邮箱地址
	Create() string

	// WaitForCode 等待验证码，返回验证码字符串
	WaitForCode(timeoutSec, intervalSec int) (string, error)

	// GetAddress 获取当前邮箱地址
	GetAddress() string
}

// moEmailAdapter 适配器，将 MoeMailProvider 包装为 TempEmailService
type moEmailAdapter struct {
	baseURL  string
	apiKey   string
	proxy    string
	provider *MoeMailProvider
}

// NewMoEmailService 创建 MoEmail 临时邮箱服务（兼容 reg_go 核心调用）
func NewMoEmailService(baseURL, apiKey string, proxyURL ...string) TempEmailService {
	proxy := ""
	if len(proxyURL) > 0 {
		proxy = proxyURL[0]
	}
	return &moEmailAdapter{
		baseURL: baseURL,
		apiKey:  apiKey,
		proxy:   proxy,
	}
}

// Create 创建临时邮箱
func (a *moEmailAdapter) Create() string {
	config := MoeMailConfig{
		Name:   "auto",
		URL:    a.baseURL,
		APIKey: a.apiKey,
	}

	// 获取可用域名
	client := NewMoeMailClientWithProxy(config, a.proxy)
	sysConfig, err := client.GetSystemConfig()
	if err != nil {
		log.Printf("[MoEmail] 获取系统配置失败: %v", err)
		return ""
	}
	if len(sysConfig.Domains) == 0 {
		log.Printf("[MoEmail] 没有可用域名")
		return ""
	}

	domain := sysConfig.Domains[0]
	name := GenerateEmailName(0)

	provider, err := NewMoeMailProviderWithProxy(config, name, 3600000, domain, a.proxy)
	if err != nil {
		log.Printf("[MoEmail] 创建邮箱失败: %v", err)
		return ""
	}

	a.provider = provider
	return provider.GetAddress()
}

// WaitForCode 等待验证码
func (a *moEmailAdapter) WaitForCode(timeout, interval int) (string, error) {
	if a.provider == nil {
		return "", nil
	}
	return a.provider.WaitForCode(timeout, interval)
}

// GetAddress 获取邮箱地址
func (a *moEmailAdapter) GetAddress() string {
	if a.provider == nil {
		return ""
	}
	return a.provider.GetAddress()
}
