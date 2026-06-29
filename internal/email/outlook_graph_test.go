package email

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
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

func TestRefreshOutlookGraphTokenFallsBackToDirectWhenProxyRefused(t *testing.T) {
	var gotClientID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotClientID = r.Form.Get("client_id")
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "direct-token"})
	}))
	defer server.Close()

	oldEndpoint := outlookGraphTokenEndpoint
	outlookGraphTokenEndpoint = server.URL + "/token"
	t.Cleanup(func() { outlookGraphTokenEndpoint = oldEndpoint })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refusedProxy := "http://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	token, err := RefreshOutlookGraphTokenWithProxy(OutlookAccount{
		Email:        "user@outlook.com",
		ClientID:     "client-id",
		RefreshToken: "refresh-token",
	}, refusedProxy)
	if err != nil {
		t.Fatal(err)
	}
	if token != "direct-token" {
		t.Fatalf("fallback token mismatch: got %q", token)
	}
	if gotClientID != "client-id" {
		t.Fatalf("direct fallback did not submit client_id, got %q", gotClientID)
	}
}

func TestGetOutlookGraphProfileFallsBackToDirectWhenProxyRefused(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "direct-token"})
		case r.URL.Path == "/me":
			if got := r.Header.Get("Authorization"); got != "Bearer direct-token" {
				t.Fatalf("unexpected authorization: %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"userPrincipalName": "primary@hotmail.com",
				"mail":              "primary@hotmail.com",
				"proxyAddresses":    []string{"SMTP:primary@hotmail.com", "smtp:alias@hotmail.com"},
				"otherMails":        []string{"other@hotmail.com"},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldEndpoint := outlookGraphTokenEndpoint
	oldBase := outlookGraphAPIBase
	outlookGraphTokenEndpoint = server.URL + "/token"
	outlookGraphAPIBase = server.URL
	t.Cleanup(func() {
		outlookGraphTokenEndpoint = oldEndpoint
		outlookGraphAPIBase = oldBase
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refusedProxy := "http://" + listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	profile, err := GetOutlookGraphProfileWithProxy(OutlookAccount{
		Email:        "alias@hotmail.com",
		ClientID:     "client-id",
		RefreshToken: "refresh-token",
	}, refusedProxy)
	if err != nil {
		t.Fatal(err)
	}
	if profile.PrimaryEmail != "primary@hotmail.com" || !profile.HasAddress("alias@hotmail.com") || !profile.HasAddress("other@hotmail.com") {
		t.Fatalf("profile fallback parsed unexpected data: %+v", profile)
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

	code, err := WaitForOTPGraphWithProxy(context.Background(), OutlookAccount{
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

func TestWaitForOTPGraphRetriesTransientTokenRefreshEOF(t *testing.T) {
	after := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	tokenAttempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			tokenAttempts++
			if tokenAttempts == 1 {
				hj, ok := w.(http.Hijacker)
				if !ok {
					t.Fatalf("test server does not support hijacking")
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					t.Fatalf("hijack failed: %v", err)
				}
				_ = conn.Close()
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "graph-access-token"})
		case strings.Contains(r.URL.Path, "/me/mailFolders/inbox/messages"):
			_ = json.NewEncoder(w).Encode(graphMessagesResponse{Value: []graphMessage{
				testGraphMessage("AWS 验证码", "2026-05-20T10:00:01Z", "你的验证码是 123456"),
			}})
		case strings.Contains(r.URL.Path, "/me/mailFolders/junkemail/messages"):
			_ = json.NewEncoder(w).Encode(graphMessagesResponse{})
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

	code, err := WaitForOTPGraphWithProxy(context.Background(), OutlookAccount{
		Email:        "user@outlook.com",
		ClientID:     "client-id",
		RefreshToken: "refresh-token",
	}, after, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if code != "123456" {
		t.Fatalf("应在 token transient EOF 后重试并获取验证码，got %q", code)
	}
	if tokenAttempts != 2 {
		t.Fatalf("expected exactly 2 token refresh attempts, got %d", tokenAttempts)
	}
}

func TestWaitForOTPGraphFiltersByRegistrationEmailWhenPresent(t *testing.T) {
	after := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "graph-access-token"})
		case strings.Contains(r.URL.Path, "/me/mailFolders/inbox/messages"):
			msg := testGraphMessage("AWS 验证码", "2026-05-20T10:00:01Z", "你的验证码是 777888")
			msg.ToRecipients = append(msg.ToRecipients, struct {
				EmailAddress struct {
					Address string `json:"address"`
				} `json:"emailAddress"`
			}{})
			msg.ToRecipients[0].EmailAddress.Address = "actual@hotmail.com"
			_ = json.NewEncoder(w).Encode(graphMessagesResponse{Value: []graphMessage{msg}})
		case strings.Contains(r.URL.Path, "/me/mailFolders/junkemail/messages"):
			_ = json.NewEncoder(w).Encode(graphMessagesResponse{})
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

	code, err := WaitForOTPGraphWithProxy(context.Background(), OutlookAccount{
		Email:             "alias@outlook.jp",
		RegistrationEmail: "actual@hotmail.com",
		ClientID:          "client-id",
		RefreshToken:      "refresh-token",
	}, after, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if code != "777888" {
		t.Fatalf("should read OTP addressed to RegistrationEmail, got %q", code)
	}
}

func TestGetOutlookGraphProfileWithProxyCollectsAliases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			fmt.Fprint(w, `{"access_token":"token"}`)
		case "/me":
			if got := r.URL.Query().Get("$select"); got != "userPrincipalName,mail,proxyAddresses,otherMails" {
				t.Fatalf("unexpected $select: %q", got)
			}
			fmt.Fprint(w, `{"userPrincipalName":"primary@hotmail.com","mail":"primary@hotmail.com","proxyAddresses":["SMTP:primary@hotmail.com","smtp:alias@outlook.jp"],"otherMails":["other@example.com"]}`)
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

	profile, err := GetOutlookGraphProfileWithProxy(OutlookAccount{ClientID: "client-id", RefreshToken: "refresh-token"}, "")
	if err != nil {
		t.Fatalf("GetOutlookGraphProfileWithProxy: %v", err)
	}
	if profile.PrimaryEmail != "primary@hotmail.com" {
		t.Fatalf("PrimaryEmail=%q", profile.PrimaryEmail)
	}
	if !profile.HasAliasData() {
		t.Fatalf("expected Graph alias fields to be marked available")
	}
	if !profile.HasAddress("alias@outlook.jp") || !profile.HasAddress("other@example.com") || !profile.HasAddress("PRIMARY@hotmail.com") {
		t.Fatalf("aliases not collected/matched correctly: %#v", profile.Aliases)
	}
}

func TestGetOutlookGraphProfileWithProxyDistinguishesMissingAliasFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			fmt.Fprint(w, `{"access_token":"token"}`)
		case "/me":
			fmt.Fprint(w, `{"userPrincipalName":"primary@hotmail.com","mail":"primary@hotmail.com"}`)
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

	profile, err := GetOutlookGraphProfileWithProxy(OutlookAccount{ClientID: "client-id", RefreshToken: "refresh-token"}, "")
	if err != nil {
		t.Fatalf("GetOutlookGraphProfileWithProxy: %v", err)
	}
	if profile.HasAliasData() {
		t.Fatalf("missing Graph alias fields should not be treated as complete alias data: %#v", profile)
	}
	if !profile.HasAddress("primary@hotmail.com") {
		t.Fatalf("primary address should still be matched: %#v", profile)
	}
}

func TestWaitForOTPGraphTimeoutDiagnosticNoRelevantMessages(t *testing.T) {
	after := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	messages := []graphMessage{
		testGraphMessage("普通邮件", "2026-05-20T10:00:01Z", "hello"),
	}
	diag := diagnoseGraphOTPMessages(messages, "alias@outlook.jp", after)
	if diag.RelevantMessages != 0 || diag.Classification != "no_relevant_messages" {
		t.Fatalf("unexpected diag: %+v", diag)
	}
}

func TestWaitForOTPGraphTimeoutDiagnosticOtherAliasOnly(t *testing.T) {
	after := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	msg := testGraphMessage("AWS 验证码", "2026-05-20T10:00:01Z", "你的验证码是 123456")
	msg.ToRecipients = append(msg.ToRecipients, struct {
		EmailAddress struct {
			Address string `json:"address"`
		} `json:"emailAddress"`
	}{})
	msg.ToRecipients[0].EmailAddress.Address = "other@outlook.jp"
	diag := diagnoseGraphOTPMessages([]graphMessage{msg}, "alias@outlook.jp", after)
	if diag.RelevantMessages != 1 || diag.OtherAliasMessages != 1 || diag.Classification != "other_alias_only" {
		t.Fatalf("unexpected diag: %+v", diag)
	}
}

func TestWaitForOTPGraphTimeoutDiagnosticTargetWithoutCode(t *testing.T) {
	after := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	msg := testGraphMessage("AWS Verify", "2026-05-20T10:00:01Z", "验证码邮件但暂无数字")
	msg.ToRecipients = append(msg.ToRecipients, struct {
		EmailAddress struct {
			Address string `json:"address"`
		} `json:"emailAddress"`
	}{})
	msg.ToRecipients[0].EmailAddress.Address = "alias@outlook.jp"
	diag := diagnoseGraphOTPMessages([]graphMessage{msg}, "alias@outlook.jp", after)
	if diag.TargetMessages != 1 || diag.TargetWithoutCode != 1 || diag.Classification != "target_without_code" {
		t.Fatalf("unexpected diag: %+v", diag)
	}
}
