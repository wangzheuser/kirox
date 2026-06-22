package email

// NewMail2MeService 创建固定使用 mail2me.co 域名的 DropMail 临时邮箱服务。
func NewMail2MeService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"mail2me.co"}
	return service
}

// NewPickMeMailService 创建固定使用 pickmemail.com 域名的 DropMail 临时邮箱服务。
func NewPickMeMailService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"pickmemail.com"}
	return service
}

// NewMaxiMailService 创建固定使用 maximail.vip 域名的 DropMail 临时邮箱服务。
func NewMaxiMailService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"maximail.vip"}
	return service
}

// NewEmlProService 创建固定使用 emlpro.com 域名的 DropMail 临时邮箱服务。
func NewEmlProService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"emlpro.com"}
	return service
}

// NewFreeMLService 创建固定使用 freeml.net 域名的 DropMail 临时邮箱服务。
func NewFreeMLService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"freeml.net"}
	return service
}

// NewEmlHubService 创建固定使用 emlhub.com 域名的 DropMail 临时邮箱服务。
func NewEmlHubService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"emlhub.com"}
	return service
}

// NewEmlTmpService 创建固定使用 emltmp.com 域名的 DropMail 临时邮箱服务。
func NewEmlTmpService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"emltmp.com"}
	return service
}

// NewMailPwrService 创建固定使用 mailpwr.com 域名的 DropMail 临时邮箱服务。
func NewMailPwrService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"mailpwr.com"}
	return service
}

// NewTenMailService 创建固定使用 10mail 系列域名的 DropMail 临时邮箱服务。
func NewTenMailService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"10mail.info", "10mail.org", "10mail.xyz"}
	return service
}

// NewDropMailMeService 创建固定使用 dropmail.me 域名的 DropMail 临时邮箱服务。
func NewDropMailMeService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"dropmail.me"}
	return service
}

// NewMimiMailService 创建固定使用 mimimail.me 域名的 DropMail 临时邮箱服务。
func NewMimiMailService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"mimimail.me"}
	return service
}

// NewPickMailService 创建固定使用 pickmail.org 域名的 DropMail 临时邮箱服务。
func NewPickMailService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"pickmail.org"}
	return service
}

// NewSpyMailService 创建固定使用 spymail.one 域名的 DropMail 临时邮箱服务。
func NewSpyMailService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"spymail.one"}
	return service
}

// NewYoMailService 创建固定使用 yomail.info 域名的 DropMail 临时邮箱服务。
func NewYoMailService(proxyURL string) *DropMailService {
	service := NewDropMailService(proxyURL)
	service.preferredDomains = []string{"yomail.info"}
	return service
}
