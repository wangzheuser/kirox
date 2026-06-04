package subscription

import (
	"errors"
	"testing"
)

func TestIsSuspendedRecognizesExistingHTTPAndTextSignals(t *testing.T) {
	cases := []string{
		"HTTP 403: forbidden",
		"HTTP 423: locked",
		"AccountSuspendedException: account suspended",
		"temporarily is suspended",
		"temporarily suspended",
		"locked your account",
		"not authorized to access this feature",
		"账号已被封禁",
	}
	for _, msg := range cases {
		if !IsSuspended(errors.New(msg)) {
			t.Fatalf("IsSuspended should recognize %q", msg)
		}
	}
}

func TestIsSuspendedRecognizesJSONReasonSignal(t *testing.T) {
	err := errors.New(`HTTP 429: {"reason":"ACCOUNT_SUSPENDED","message":"temporarily suspended by upstream"}`)

	if !IsSuspended(err) {
		t.Fatalf("IsSuspended should recognize JSON reason responses as suspended")
	}
}

func TestIsSuspendedIgnoresUnrelatedErrors(t *testing.T) {
	err := errors.New(`HTTP 500: {"message":"service unavailable"}`)

	if IsSuspended(err) {
		t.Fatalf("IsSuspended should ignore unrelated errors")
	}
}
