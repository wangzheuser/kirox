package email

import "testing"

func TestDropMailDomainChannelConstructorsUseExpectedDomain(t *testing.T) {
	cases := []struct {
		name    string
		service *DropMailService
		domain  string
	}{
		{name: "mail2me", service: NewMail2MeService(""), domain: "mail2me.co"},
		{name: "pickmemail", service: NewPickMeMailService(""), domain: "pickmemail.com"},
		{name: "maximail", service: NewMaxiMailService(""), domain: "maximail.vip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.service.preferredDomains[0]; got != tc.domain {
				t.Fatalf("preferred domain=%q, want %q", got, tc.domain)
			}
		})
	}
}

func TestPreferredDropMailDomainIDAcceptsCustomPreference(t *testing.T) {
	domains := []dropMailDomain{{ID: "d1", Name: "mailtowin.com"}, {ID: "d2", Name: "pickmemail.com"}}
	if got := preferredDropMailDomainID(domains, []string{"pickmemail.com"}); got != "d2" {
		t.Fatalf("domain id=%q, want d2", got)
	}
}
