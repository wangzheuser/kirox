package email

// MailToWinService 是 mailtowin.com 域名的零配置临时邮箱渠道。
// 底层使用匿名临时邮箱会话收信，但作为独立 provider 暴露，便于在注册任务中单独选择该高通过率域名。
type MailToWinService struct {
	*DropMailService
}

// NewMailToWinService 创建 mailtowin.com 临时邮箱服务。
func NewMailToWinService(proxyURL string) *MailToWinService {
	return &MailToWinService{DropMailService: NewDropMailService(proxyURL)}
}
