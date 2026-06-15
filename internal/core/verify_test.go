package core

import (
	"bytes"
	"log"
	"strings"
	"testing"
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

func TestShouldVerifyModelsFollowsConfigSwitch(t *testing.T) {
	if shouldVerifyModels(nil) {
		t.Fatalf("nil config should default ListAvailableModels verification off")
	}
	if shouldVerifyModels(&Config{}) {
		t.Fatalf("zero-value config should default ListAvailableModels verification off")
	}
	if !shouldVerifyModels(&Config{VerifyModelsEnabled: true}) {
		t.Fatalf("VerifyModelsEnabled=true should enable ListAvailableModels verification")
	}
}
