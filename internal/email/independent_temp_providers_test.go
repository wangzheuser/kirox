package email

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSmailProCreateAndWaitForCode(t *testing.T) {
	var apiURL string
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/payload" {
			t.Fatalf("unexpected app path %s", r.URL.Path)
		}
		target := r.URL.Query().Get("url")
		if !strings.HasPrefix(target, apiURL) {
			t.Fatalf("payload url=%q, want api base %q", target, apiURL)
		}
		payload := strings.TrimPrefix(strings.TrimPrefix(target, apiURL), "/")
		if email := r.URL.Query().Get("email"); email != "" {
			payload += "|" + email
		}
		if mid := r.URL.Query().Get("mid"); mid != "" {
			payload += "|" + mid
		}
		_, _ = w.Write([]byte(payload))
	}))
	defer app.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/create":
			if r.URL.Query().Get("payload") != "create" {
				t.Fatalf("create payload=%q", r.URL.Query().Get("payload"))
			}
			_, _ = w.Write([]byte(`{"email":"kiro@smail.test","expired_at":1782380081}`))
		case "/inbox":
			if r.URL.Query().Get("payload") != "inbox|kiro@smail.test" {
				t.Fatalf("inbox payload=%q", r.URL.Query().Get("payload"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"messages": []map[string]interface{}{{"mid": "m1", "subject": "Verify"}},
			})
		case "/message":
			if r.URL.Query().Get("payload") != "message|kiro@smail.test|m1" {
				t.Fatalf("message payload=%q", r.URL.Query().Get("payload"))
			}
			_, _ = w.Write([]byte(`{"body":"Your AWS verification code is 123456","textSubject":"Verify"}`))
		default:
			t.Fatalf("unexpected api path %s", r.URL.Path)
		}
	}))
	defer api.Close()
	apiURL = api.URL

	oldApp, oldAPI := smailProAppBaseURL, smailProAPIBaseURL
	smailProAppBaseURL, smailProAPIBaseURL = app.URL, api.URL
	t.Cleanup(func() { smailProAppBaseURL, smailProAPIBaseURL = oldApp, oldAPI })

	service := NewSmailProService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError: %v", err)
	}
	if address != "kiro@smail.test" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode: %v", err)
	}
	if code != "123456" {
		t.Fatalf("code=%q", code)
	}
}

func TestTempMailboxCreateAndWaitForCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<meta name="csrf-token" content="csrf123">`))
		case "/get_messages":
			if r.FormValue("_token") != "csrf123" {
				t.Fatalf("token=%q", r.FormValue("_token"))
			}
			_, _ = w.Write([]byte(`{"status":true,"mailbox":"kiro@tempmailbox.test","email_token":"tok","messages":[{"id":"m1","from":"AWS","from_email":"no-reply@example.com","subject":"Verify"}]}`))
		case "/view/m1":
			_, _ = w.Write([]byte(`Your AWS verification code is 654321`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBase := tempMailboxBaseURL
	tempMailboxBaseURL = server.URL
	t.Cleanup(func() { tempMailboxBaseURL = oldBase })

	service := NewTempMailboxService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError: %v", err)
	}
	if address != "kiro@tempmailbox.test" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode: %v", err)
	}
	if code != "654321" {
		t.Fatalf("code=%q", code)
	}
}

func TestGoneBoxCreateAndWaitForCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/inboxes":
			var req struct {
				Domain string `json:"domain"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("create json: %v", err)
			}
			if req.Domain != "gonebox.email" {
				t.Fatalf("domain=%q", req.Domain)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": map[string]interface{}{
					"id":      "ibox1",
					"address": "kiro@gonebox.email",
					"domain":  "gonebox.email",
				},
			})
		case "/api/v1/inboxes/kiro%40gonebox.email/messages", "/api/v1/inboxes/kiro@gonebox.email/messages":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data": map[string]interface{}{
					"messages": []map[string]interface{}{{"id": "m1", "subject": "Verify", "from_address": "AWS"}},
				},
			})
		case "/api/v1/messages/m1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"data":    map[string]interface{}{"body_text": "Your AWS verification code is 334455"},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBase := goneBoxAPIBaseURL
	goneBoxAPIBaseURL = server.URL + "/api/v1"
	t.Cleanup(func() { goneBoxAPIBaseURL = oldBase })

	service := NewGoneBoxService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError: %v", err)
	}
	if address != "kiro@gonebox.email" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode: %v", err)
	}
	if code != "334455" {
		t.Fatalf("code=%q", code)
	}
}

func TestOpenInboxCreateAndWaitForCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/inbox":
			var req struct {
				Fingerprint string `json:"fingerprint"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("create json: %v", err)
			}
			if len(req.Fingerprint) != 64 {
				t.Fatalf("fingerprint len=%d", len(req.Fingerprint))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":    "ibox1",
				"email": "kiro@openinbox.test",
			})
		case "/api/emails/inbox/ibox1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"emails": []map[string]interface{}{{"id": "m1", "subject": "Verify", "from": "AWS"}},
			})
		case "/api/emails/m1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":       "m1",
				"textBody": "Your AWS verification code is 556677",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBase := openInboxAPIBaseURL
	openInboxAPIBaseURL = server.URL + "/api"
	t.Cleanup(func() { openInboxAPIBaseURL = oldBase })

	service := NewOpenInboxService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError: %v", err)
	}
	if address != "kiro@openinbox.test" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode: %v", err)
	}
	if code != "556677" {
		t.Fatalf("code=%q", code)
	}
}

func TestBlinkBoxCreateAndWaitForCode(t *testing.T) {
	homeSnapshot := `{"data":{"emailAddress":"","addressHash":"","perPage":20,"paginators":[{"emailsPage":1},{"s":"arr"}]},"memo":{"id":"cmp1","name":"unified-inbox","path":"\/","method":"GET","children":[],"scripts":[],"assets":[],"errors":[],"locale":"en"},"checksum":"home"}`
	activeSnapshot := `{"data":{"emailAddress":"kiro@fontdle.com","addressHash":"abc778899def","perPage":20,"paginators":[{"emailsPage":1},{"s":"arr"}]},"memo":{"id":"cmp2","name":"unified-inbox","path":"\/","method":"GET","children":[],"scripts":[],"assets":[],"errors":[],"locale":"en"},"checksum":"active"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<div wire:snapshot="` + html.EscapeString(homeSnapshot) + `" wire:effects="{}" wire:id="cmp1"></div><script src="/livewire/livewire.js" data-csrf="csrf123" data-update-uri="/livewire/update"></script>`))
		case "/livewire/update":
			var req struct {
				Token      string `json:"_token"`
				Components []struct {
					Calls []struct {
						Method string        `json:"method"`
						Params []interface{} `json:"params"`
					} `json:"calls"`
				} `json:"components"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("livewire json: %v", err)
			}
			if req.Token != "csrf123" {
				t.Fatalf("token=%q", req.Token)
			}
			if len(req.Components) != 1 || len(req.Components[0].Calls) != 1 {
				t.Fatalf("calls=%+v", req.Components)
			}
			switch req.Components[0].Calls[0].Method {
			case "generateRandomEmail":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"components": []map[string]interface{}{{
						"snapshot": homeSnapshot,
						"effects": map[string]interface{}{
							"dispatches": []map[string]interface{}{{"name": "emailGenerated", "params": map[string]interface{}{"address": "kiro@fontdle.com"}}},
						},
					}},
				})
			case "setActiveEmail":
				if len(req.Components[0].Calls[0].Params) != 1 || req.Components[0].Calls[0].Params[0] != "kiro@fontdle.com" {
					t.Fatalf("setActiveEmail params=%+v", req.Components[0].Calls[0].Params)
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"components": []map[string]interface{}{{
						"snapshot": activeSnapshot,
						"effects":  map[string]interface{}{},
					}},
				})
			case "loadEmails":
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"components": []map[string]interface{}{{
						"snapshot": activeSnapshot,
						"effects": map[string]interface{}{
							"html": `<article><span style="color: #555555">AWS logo</span><div>Verification code:</div><div class="code">112233</div></article>`,
						},
					}},
				})
			default:
				t.Fatalf("unexpected method %s", req.Components[0].Calls[0].Method)
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	oldBase := blinkBoxBaseURL
	blinkBoxBaseURL = server.URL
	t.Cleanup(func() { blinkBoxBaseURL = oldBase })

	service := NewBlinkBoxService("")
	address, err := service.CreateWithError()
	if err != nil {
		t.Fatalf("CreateWithError: %v", err)
	}
	if address != "kiro@fontdle.com" {
		t.Fatalf("address=%q", address)
	}
	code, err := service.WaitForCode(1, 1)
	if err != nil {
		t.Fatalf("WaitForCode: %v", err)
	}
	if code != "112233" {
		t.Fatalf("code=%q", code)
	}
}
