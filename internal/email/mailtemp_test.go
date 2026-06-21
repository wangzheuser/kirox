package email

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withMailTempTestServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	oldBase := mailTempBaseURL
	mailTempBaseURL = server.URL
	t.Cleanup(func() { mailTempBaseURL = oldBase })
}

func TestMailTempCreateParsesGeneratedAddress(t *testing.T) {
	withMailTempTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("path=%q, want /", r.URL.Path)
		}
		_, _ = w.Write([]byte(`<span id="email_ch_text">fayettecpl@himacreative.id</span>`))
	})

	service := NewMailTempService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError returned error: %v", err)
	}
	if address != "fayettecpl@himacreative.id" {
		t.Fatalf("address=%q", address)
	}
	if service.GetAddress() != address {
		t.Fatalf("GetAddress=%q, want %q", service.GetAddress(), address)
	}
}

func TestMailTempWaitForCodeExtractsCodeFromInboxPage(t *testing.T) {
	withMailTempTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/himacreative.id/fayettecpl") {
			t.Fatalf("path=%q, want mailbox path", r.URL.Path)
		}
		_, _ = w.Write([]byte(`<html><body>Verify your AWS Builder ID. Your verification code is <b>654321</b>.</body></html>`))
	})

	service := NewMailTempService("")
	service.address = "fayettecpl@himacreative.id"
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode returned error: %v", err)
	}
	if code != "654321" {
		t.Fatalf("code=%q, want 654321", code)
	}
}

func TestMailTempCodeFromDeliveredInboxHTML(t *testing.T) {
	html := `<title>Verify your AWS Builder ID email address - Temp Mail</title>
<div class="mess_bodiyy">
<h1>Verify your AWS Builder ID email address</h1>
<div class="label">Verification code:</div>
<div class="code" style="font-size:36px">694044</div>
</div>`

	if code := mailTempCodeFromHTML(html); code != "694044" {
		t.Fatalf("code=%q, want 694044", code)
	}
}
