package email

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMinuteInboxCreateAndWaitForCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<script>const CSRF = "csrf123"</script>`))
		case "/index/index":
			if r.URL.Query().Get("csrf_token") != "csrf123" {
				t.Fatalf("csrf=%q", r.URL.Query().Get("csrf_token"))
			}
			_, _ = w.Write([]byte(`{"email":"kiro@minute.test"}`))
		case "/index/refresh":
			_, _ = w.Write([]byte(`[{"id":"m1"}]`))
		case "/index/email":
			if r.FormValue("id") != "m1" {
				t.Fatalf("id=%q", r.FormValue("id"))
			}
			_, _ = w.Write([]byte(`{"body":"Your AWS verification code is 778899"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := newSessionTempSiteService("", server.URL, "MinuteInbox")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError: %v", err)
	}
	if address != "kiro@minute.test" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode: %v", err)
	}
	if code != "778899" {
		t.Fatalf("code=%q", code)
	}
}
