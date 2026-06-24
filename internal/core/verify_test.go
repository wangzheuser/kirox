package core

import (
	"bytes"
	"io"
	"log"
	"strings"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

func captureVerifyLog(t *testing.T, fn func() endpointResult) (endpointResult, string) {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })
	res := fn()
	return res, buf.String()
}

func TestCheckEndpointResponseTreats403AsSuspended(t *testing.T) {
	res := checkEndpointResponse("https://q.us-east-1.amazonaws.com/getUsageLimits?origin=AI_EDITOR", 403, []byte(`{"message":"forbidden"}`))

	if !res.suspended {
		t.Fatalf("HTTP 403 should be classified as suspended: %+v", res)
	}
}

func TestCheckEndpointResponseModels403IsOrdinaryFailure(t *testing.T) {
	res := checkEndpointResponse("https://q.us-east-1.amazonaws.com/ListAvailableModels?origin=AI_EDITOR", 403, []byte(`{"message":"forbidden"}`))

	if res.suspended {
		t.Fatalf("models HTTP 403 should not mark account suspended: %+v", res)
	}
	if res.ok {
		t.Fatalf("models HTTP 403 should not be ok: %+v", res)
	}
}

func TestCheckEndpointResponseModels403SuspendedBodyMarksSuspended(t *testing.T) {
	body := []byte(`{"message":"Your User ID temporarily is suspended. We've locked your account as a security precaution."}`)

	res := checkEndpointResponse("https://q.us-east-1.amazonaws.com/ListAvailableModels?origin=AI_EDITOR", 403, body)

	if !res.suspended {
		t.Fatalf("models 403 with suspended/locked body should mark account suspended: %+v", res)
	}
}

func TestCheckEndpointResponseTreatsReasonJSONAsSuspendedAndLogsFullBody(t *testing.T) {
	body := []byte(`{"reason":"ACCOUNT_SUSPENDED","message":"temporarily suspended by upstream"}`)

	res, logs := captureVerifyLog(t, func() endpointResult {
		return checkEndpointResponse("https://q.us-east-1.amazonaws.com/getUsageLimits?origin=AI_EDITOR", 429, body)
	})

	if !res.suspended {
		t.Fatalf("non-2xx response with JSON reason should be classified as suspended: %+v", res)
	}
	for _, want := range []string{"usage", "429", "ACCOUNT_SUSPENDED", string(body)} {
		if !strings.Contains(logs, want) {
			t.Fatalf("reason suspension log should contain %q; logs=%q", want, logs)
		}
	}
}

func TestCheckEndpointResponseWithoutReasonJSONIsOrdinaryFailure(t *testing.T) {
	body := []byte(`{"message":"service unavailable"}`)

	res := checkEndpointResponse("https://q.us-east-1.amazonaws.com/getUsageLimits?origin=AI_EDITOR", 503, body)

	if res.suspended {
		t.Fatalf("non-2xx response without reason should not be classified as suspended: %+v", res)
	}
	if res.ok {
		t.Fatalf("non-2xx response should not be ok: %+v", res)
	}
}

func TestCheckEndpointResponseReasonJSONOnlySuspendsUsageEndpoint(t *testing.T) {
	body := []byte(`{"reason":"MODEL_ENDPOINT_REASON"}`)

	res := checkEndpointResponse("https://q.us-east-1.amazonaws.com/ListAvailableModels?origin=AI_EDITOR", 429, body)

	if res.suspended {
		t.Fatalf("reason JSON should only classify getUsageLimits as suspended: %+v", res)
	}
}

func TestEndpointBodySnippetTruncatesAndScrubs(t *testing.T) {
	body := []byte(`{"message":"forbidden","url":"https://example.com/very/secret/path","padding":"` + strings.Repeat("x", 400) + `"}`)
	got := endpointBodySnippet(body)
	if strings.Contains(got, "https://example.com/very/secret/path") {
		t.Fatalf("endpoint body snippet should scrub URLs: %s", got)
	}
	if len(got) > 220 {
		t.Fatalf("endpoint body snippet should be capped, len=%d body=%s", len(got), got)
	}
}

type verifyRoundTrip func(req *fhttp.Request) (*fhttp.Response, error)

func (f verifyRoundTrip) Do(req *fhttp.Request) (*fhttp.Response, error) { return f(req) }

func verifyHTTPResponse(status int, body string) *fhttp.Response {
	return &fhttp.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestVerifyAliveReturnsFailureWhenModels403(t *testing.T) {
	orig := newVerifyHTTPClient
	defer func() { newVerifyHTTPClient = orig }()

	newVerifyHTTPClient = func(cfg *Config, chromeVer string) verifyHTTPClient {
		return verifyRoundTrip(func(req *fhttp.Request) (*fhttp.Response, error) {
			switch {
			case strings.Contains(req.URL.String(), "/token"):
				return verifyHTTPResponse(200, `{"accessToken":"access-token","expiresIn":3600}`), nil
			case strings.Contains(req.URL.String(), "getUsageLimits"):
				return verifyHTTPResponse(200, `{"userInfo":{"email":"user@example.com"},"subscriptionInfo":{"subscriptionTitle":"Free"},"usageBreakdownList":[]}`), nil
			case strings.Contains(req.URL.String(), "ListAvailableModels"):
				return verifyHTTPResponse(403, `{"message":"forbidden"}`), nil
			default:
				t.Fatalf("unexpected request URL: %s", req.URL.String())
				return nil, nil
			}
		})
	}

	res := (&Registrar{Cfg: &Config{}, ClientID: "client", ClientSecret: "secret"}).VerifyAlive(map[string]interface{}{"refreshToken": "refresh"})

	if alive, _ := res["alive"].(bool); alive {
		t.Fatalf("models 403 should fail second verification: %#v", res)
	}
	if suspended, _ := res["suspended"].(bool); suspended {
		t.Fatalf("models 403 should not mark suspended: %#v", res)
	}
	if got, _ := res["error"].(string); got != "models query failed: 403" {
		t.Fatalf("unexpected models failure error: %#v", res)
	}
}

func TestVerifyAliveUsesConfiguredOIDCAndQBase(t *testing.T) {
	orig := newVerifyHTTPClient
	defer func() { newVerifyHTTPClient = orig }()

	var seen []string
	newVerifyHTTPClient = func(cfg *Config, chromeVer string) verifyHTTPClient {
		return verifyRoundTrip(func(req *fhttp.Request) (*fhttp.Response, error) {
			seen = append(seen, req.URL.String())
			switch {
			case strings.Contains(req.URL.String(), "/token"):
				return verifyHTTPResponse(200, `{"accessToken":"access-token","expiresIn":3600}`), nil
			case strings.Contains(req.URL.String(), "getUsageLimits"):
				return verifyHTTPResponse(200, `{"userInfo":{"email":"user@example.com"},"subscriptionInfo":{"subscriptionTitle":"Free"},"usageBreakdownList":[]}`), nil
			case strings.Contains(req.URL.String(), "ListAvailableModels"):
				return verifyHTTPResponse(200, `{"models":[]}`), nil
			default:
				t.Fatalf("unexpected request URL: %s", req.URL.String())
				return nil, nil
			}
		})
	}

	cfg := &Config{OIDCBase: "http://127.0.0.1:18080/oidc", QBase: "http://127.0.0.1:18080/q"}
	res := (&Registrar{Cfg: cfg, ClientID: "client", ClientSecret: "secret"}).VerifyAlive(map[string]interface{}{"refreshToken": "refresh"})
	if alive, _ := res["alive"].(bool); !alive {
		t.Fatalf("expected local verification to pass: %#v", res)
	}
	if len(seen) < 3 {
		t.Fatalf("expected token, usage, and models requests, got %v", seen)
	}
	if !strings.HasPrefix(seen[0], "http://127.0.0.1:18080/oidc/token") {
		t.Fatalf("token request did not use configured OIDCBase: %v", seen)
	}
	if !strings.HasPrefix(seen[1], "http://127.0.0.1:18080/q/getUsageLimits") {
		t.Fatalf("usage request did not use configured QBase: %v", seen)
	}
	if !strings.HasPrefix(seen[2], "http://127.0.0.1:18080/q/ListAvailableModels") {
		t.Fatalf("models request did not use configured QBase: %v", seen)
	}
}
