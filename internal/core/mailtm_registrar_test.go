package core

import (
	"testing"

	"reg_go/internal/email"
)

func TestStep3EmailCreatesMailTMService(t *testing.T) {
	oldFactory := newMailTMTempEmailService
	service := &fakeTempEmailService{address: "user@web-library.net"}
	newMailTMTempEmailService = func(proxyURL string) email.TempEmailService {
		if proxyURL != "http://mail-proxy" {
			t.Fatalf("proxyURL=%q, want http://mail-proxy", proxyURL)
		}
		return service
	}
	t.Cleanup(func() { newMailTMTempEmailService = oldFactory })

	r := &Registrar{Cfg: &Config{EmailProvider: "mailtm", EmailProxy: "http://mail-proxy"}}
	if err := r.Step3Email(); err != nil {
		t.Fatalf("Step3Email returned error: %v", err)
	}
	if r.Email != "user@web-library.net" || r.EmailSvc != service {
		t.Fatalf("Step3Email should bind generated mail.tm service, email=%q svc=%#v", r.Email, r.EmailSvc)
	}
	if service.createCalls != 1 {
		t.Fatalf("mail.tm service should create mailbox once, createCalls=%d", service.createCalls)
	}
}
