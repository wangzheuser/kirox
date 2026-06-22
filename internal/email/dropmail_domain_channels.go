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
