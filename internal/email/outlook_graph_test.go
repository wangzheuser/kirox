package email

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseOutlookLinesSupportsClientRefreshDash4(t *testing.T) {
	accounts := ParseOutlookLines("user@outlook.com----Password!----client-id----refresh-token")
	if len(accounts) != 1 {
		t.Fatalf("应解析到 1 个账号: got %d", len(accounts))
	}
	acc := accounts[0]
	if acc.Email != "user@outlook.com" || acc.Password != "Password!" || acc.ClientID != "client-id" || acc.RefreshToken != "refresh-token" {
		t.Fatalf("账号字段解析错误: %+v", acc)
	}
}

func TestRefreshOutlookGraphTokenUsesCommonEndpointAndGraphScopes(t *testing.T) {
	var gotScope string
	var gotClientID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotScope = r.Form.Get("scope")
		gotClientID = r.Form.Get("client_id")
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "graph-access-token"})
	}))
	defer server.Close()

	oldEndpoint := outlookGraphTokenEndpoint
	outlookGraphTokenEndpoint = server.URL + "/token"
	t.Cleanup(func() { outlookGraphTokenEndpoint = oldEndpoint })

	token, err := RefreshOutlookGraphTokenWithProxy(OutlookAccount{
		Email:        "user@outlook.com",
		ClientID:     "client-id",
		RefreshToken: "refresh-token",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if token != "graph-access-token" {
		t.Fatalf("access_token 读取错误: got %q", token)
	}
	if gotClientID != "client-id" {
		t.Fatalf("client_id 未正确提交: got %q", gotClientID)
	}
	if gotScope != outlookGraphScope {
		t.Fatalf("Graph scope 不正确: got %q", gotScope)
	}
}

func TestWaitForOTPGraphExtractsCodeAndIgnoresOldMessages(t *testing.T) {
	after := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "graph-access-token"})
		case strings.Contains(r.URL.Path, "/me/mailFolders/inbox/messages"):
			_ = json.NewEncoder(w).Encode(graphMessagesResponse{Value: []graphMessage{
				testGraphMessage("旧验证码 111111", "2026-05-20T09:59:00Z", "111111"),
			}})
		case strings.Contains(r.URL.Path, "/me/mailFolders/junkemail/messages"):
			_ = json.NewEncoder(w).Encode(graphMessagesResponse{Value: []graphMessage{
				testGraphMessage("AWS 验证码", "2026-05-20T10:00:01Z", "你的验证码是 654321"),
			}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldEndpoint := outlookGraphTokenEndpoint
	oldAPIBase := outlookGraphAPIBase
	outlookGraphTokenEndpoint = server.URL + "/token"
	outlookGraphAPIBase = server.URL
	t.Cleanup(func() {
		outlookGraphTokenEndpoint = oldEndpoint
		outlookGraphAPIBase = oldAPIBase
	})

	code, err := WaitForOTPGraphWithProxy(OutlookAccount{
		Email:        "user@outlook.com",
		ClientID:     "client-id",
		RefreshToken: "refresh-token",
	}, after, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if code != "654321" {
		t.Fatalf("应提取新邮件验证码: got %q", code)
	}
}

func testGraphMessage(subject, receivedAt, bodyPreview string) graphMessage {
	var msg graphMessage
	msg.Subject = subject
	msg.ReceivedDateTime = receivedAt
	msg.BodyPreview = bodyPreview
	msg.Body.Content = bodyPreview
	return msg
}
