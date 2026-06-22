package email

import "testing"

func TestDropMailDomainChannelConstructorsUseExpectedDomain(t *testing.T) {
	cases := []struct {
		name    string
		service *DropMailService
		domains []string
	}{
		{name: "mail2me", service: NewMail2MeService(""), domains: []string{"mail2me.co"}},
		{name: "pickmemail", service: NewPickMeMailService(""), domains: []string{"pickmemail.com"}},
		{name: "maximail", service: NewMaxiMailService(""), domains: []string{"maximail.vip"}},
		{name: "emlpro", service: NewEmlProService(""), domains: []string{"emlpro.com"}},
		{name: "freeml", service: NewFreeMLService(""), domains: []string{"freeml.net"}},
		{name: "emlhub", service: NewEmlHubService(""), domains: []string{"emlhub.com"}},
		{name: "emltmp", service: NewEmlTmpService(""), domains: []string{"emltmp.com"}},
		{name: "mailpwr", service: NewMailPwrService(""), domains: []string{"mailpwr.com"}},
		{name: "tenmail", service: NewTenMailService(""), domains: []string{"10mail.info", "10mail.org", "10mail.xyz"}},
		{name: "dropmail_me", service: NewDropMailMeService(""), domains: []string{"dropmail.me"}},
		{name: "mimimail", service: NewMimiMailService(""), domains: []string{"mimimail.me"}},
		{name: "pickmail", service: NewPickMailService(""), domains: []string{"pickmail.org"}},
		{name: "spymail", service: NewSpyMailService(""), domains: []string{"spymail.one"}},
		{name: "yomail", service: NewYoMailService(""), domains: []string{"yomail.info"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.service.preferredDomains) != len(tc.domains) {
				t.Fatalf("preferred domains=%v, want %v", tc.service.preferredDomains, tc.domains)
			}
			for i, want := range tc.domains {
				if got := tc.service.preferredDomains[i]; got != want {
					t.Fatalf("preferred domains=%v, want %v", tc.service.preferredDomains, tc.domains)
				}
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
