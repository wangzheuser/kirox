package logutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactMasksRegistrationSecretsAndPreservesLabels(t *testing.T) {
	input := `email=user@example.com 验证码: 123456 user_code=ABCD-EFGH workflowState=state-value regCode=reg-value code=auth-value accessToken=access-value refreshToken:"refresh-value" password=secret-value https://proxy-user:proxy-pass@proxy.example:443 errorCode:"BLOCKED"`
	got := Redact(input)
	for _, secret := range []string{"user@example.com", "123456", "ABCD-EFGH", "state-value", "reg-value", "auth-value", "access-value", "refresh-value", "secret-value", "proxy-pass"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sensitive value %q leaked in %q", secret, got)
		}
	}
	for _, want := range []string{"email=u***@example.com", `errorCode:"BLOCKED"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in %q", want, got)
		}
	}
}

func TestRedactFileRewritesExistingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("email=user@example.com user_code=ABCD-EFGH\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RedactFile(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "email=u***@example.com user_code=***\n" {
		t.Fatalf("unexpected redacted file: %q", got)
	}
}

func TestRedactFileOnceRescansOnlyWhenLogChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("user_code=FIRST\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RedactFileOnce(path); err != nil {
		t.Fatal(err)
	}
	markerBefore, err := os.ReadFile(path + redactionMarkerSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if err := RedactFileOnce(path); err != nil {
		t.Fatal(err)
	}
	markerAfter, err := os.ReadFile(path + redactionMarkerSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if string(markerBefore) != string(markerAfter) {
		t.Fatalf("unchanged log should retain its fingerprint marker")
	}
	if err := os.WriteFile(path, []byte("user_code=SECOND\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RedactFileOnce(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "user_code=***\n" {
		t.Fatalf("changed log was not rescanned: %q", got)
	}
}
