package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDropMailCreateUsesPreferredDomain(t *testing.T) {
	var domainListed, sessionIntroduced bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/graphql/test-token" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		query := payload["query"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(query, "domains"):
			domainListed = true
			_, _ = w.Write([]byte(`{"data":{"domains":[{"id":"d1","name":"10mail.xyz"},{"id":"d2","name":"mailtowin.com"}]}}`))
		case strings.Contains(query, "introduceSession"):
			sessionIntroduced = true
			vars := payload["variables"].(map[string]interface{})
			input := vars["input"].(map[string]interface{})
			if got := input["domainId"]; got != "d2" {
				t.Fatalf("domainId=%v, want d2", got)
			}
			_, _ = w.Write([]byte(`{"data":{"introduceSession":{"id":"s1","addresses":[{"address":"user@mailtowin.com"}]}}}`))
		default:
			t.Fatalf("unexpected query: %s", query)
		}
	}))
	defer server.Close()

	service := NewDropMailService("")
	service.baseURL = server.URL + "/api/graphql"
	service.tokenGenerator = func() (string, error) { return "test-token", nil }

	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError error: %v", err)
	}
	if address != "user@mailtowin.com" {
		t.Fatalf("address=%q", address)
	}
	if !domainListed || !sessionIntroduced {
		t.Fatalf("expected domains and introduceSession calls, domains=%v session=%v", domainListed, sessionIntroduced)
	}
}

func TestDropMailWaitForCodeSkipsSESPlaceholderZeroes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		query := payload["query"].(string)
		if !strings.Contains(query, "session") {
			t.Fatalf("unexpected query: %s", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"session":{"mails":[{"id":"m1","fromAddr":"010001-test-000000@amazonses.com","headerFrom":"no-reply@signin.aws","headerSubject":"验证您的 AWS 构建者 ID 电子邮件地址","text":"验证码：: 454677\n此验证码将在发送后 30 分钟过期。","html":""}]}}}`))
	}))
	defer server.Close()

	service := NewDropMailService("")
	service.baseURL = server.URL + "/api/graphql"
	service.token = "test-token"
	service.sessionID = "s1"
	service.address = "user@mailtowin.com"

	code, err := service.WaitForCode(2, 1)
	if err != nil {
		t.Fatalf("WaitForCode error: %v", err)
	}
	if code != "454677" {
		t.Fatalf("code=%q, want 454677", code)
	}
}
