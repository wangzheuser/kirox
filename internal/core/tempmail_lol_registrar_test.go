package core

import (
	"testing"

	"reg_go/internal/email"
)

func TestStep3EmailCreatesTempMailLOLService(t *testing.T) {
	oldFactory := newTempMailLOLTempEmailService
	service := &fakeTempEmailService{address: "user@random.example"}
	newTempMailLOLTempEmailService = func(proxyURL string) email.TempEmailService {
		if proxyURL != "http://mail-proxy" {
			t.Fatalf("proxyURL=%q, want http://mail-proxy", proxyURL)
		}
		return service
	}
	t.Cleanup(func() { newTempMailLOLTempEmailService = oldFactory })

	r := &Registrar{Cfg: &Config{EmailProvider: "tempmail_lol", EmailProxy: "http://mail-proxy"}}
	if err := r.Step3Email(); err != nil {
		t.Fatalf("Step3Email returned error: %v", err)
	}
	if r.Email != "user@random.example" || r.EmailSvc != service {
		t.Fatalf("Step3Email should bind generated TempMail.lol service, email=%q svc=%#v", r.Email, r.EmailSvc)
	}
}
