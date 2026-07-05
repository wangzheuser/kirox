package email

import (
	"net/http"

	tls_client "github.com/bogdanfinn/tls-client"
)

func closeHTTPClientIdleConnections(client *http.Client) {
	if client != nil {
		client.CloseIdleConnections()
	}
}

func closeTLSClientIdleConnections(client tls_client.HttpClient) {
	if client != nil {
		client.CloseIdleConnections()
	}
}

// CloseIdleConnections closes idle HTTP connections owned by MoeMailClient.
func (c *MoeMailClient) CloseIdleConnections() {
	if c != nil {
		closeHTTPClientIdleConnections(c.client)
	}
}

// CloseIdleConnections closes idle HTTP connections owned by MoeMailProvider.
func (p *MoeMailProvider) CloseIdleConnections() {
	if p != nil && p.client != nil {
		p.client.CloseIdleConnections()
	}
}

// CloseIdleConnections closes idle HTTP connections owned by CloudMailClient.
func (c *CloudMailClient) CloseIdleConnections() {
	if c != nil {
		closeHTTPClientIdleConnections(c.client)
	}
}

// CloseIdleConnections closes idle HTTP connections owned by CloudMailProvider.
func (p *CloudMailProvider) CloseIdleConnections() {
	if p != nil && p.client != nil {
		p.client.CloseIdleConnections()
	}
}

func (a *moEmailAdapter) CloseIdleConnections() {
	if a != nil && a.provider != nil {
		a.provider.CloseIdleConnections()
	}
}

func (a *cloudMailAdapter) CloseIdleConnections() {
	if a != nil && a.provider != nil {
		a.provider.CloseIdleConnections()
	}
}

func (s *MailGWService) CloseIdleConnections() {
	if s != nil {
		closeTLSClientIdleConnections(s.client)
	}
}

func (s *EmailnatorService) CloseIdleConnections() {
	if s != nil {
		closeTLSClientIdleConnections(s.client)
	}
}

func (s *MailporaryService) CloseIdleConnections() {
	if s != nil {
		closeTLSClientIdleConnections(s.client)
	}
}

func (s *GuerrillaMailService) CloseIdleConnections() {
	if s != nil {
		closeTLSClientIdleConnections(s.client)
	}
}

func (s *InboxKittenService) CloseIdleConnections() {
	if s != nil {
		closeTLSClientIdleConnections(s.client)
	}
}

func (s *TempMailLOLService) CloseIdleConnections() {
	if s != nil {
		closeTLSClientIdleConnections(s.client)
	}
}

func (s *FreeCustomService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *MailTempService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *DropMailService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *MailCatchService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *TempMailoService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
		closeHTTPClientIdleConnections(s.directClient)
	}
}

func (s *GeneratorEmailService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *TempMailIOService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *InboxesService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *TempMailPlusService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *SessionTempSiteService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *SmailProService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *TempMailboxService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *GoneBoxService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *OpenInboxService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}

func (s *BlinkBoxService) CloseIdleConnections() {
	if s != nil {
		closeHTTPClientIdleConnections(s.client)
	}
}
