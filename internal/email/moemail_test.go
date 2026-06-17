package email

import "testing"

func TestResolveMoeMailDomainRejectsUnavailableSelectedDomain(t *testing.T) {
	_, err := resolveMoeMailDomain("wqpnode.filegear-sg.me", []string{"public-temp.example"})
	if err == nil {
		t.Fatalf("selected MoeMail domain should not silently fall back to another configured domain")
	}
}

func TestResolveMoeMailDomainKeepsSelectedDomainWhenAvailable(t *testing.T) {
	got, err := resolveMoeMailDomain("wqpnode.filegear-sg.me", []string{"public-temp.example", "wqpnode.filegear-sg.me"})
	if err != nil {
		t.Fatalf("selected available MoeMail domain should be accepted: %v", err)
	}
	if got != "wqpnode.filegear-sg.me" {
		t.Fatalf("resolved domain = %q, want selected domain", got)
	}
}
