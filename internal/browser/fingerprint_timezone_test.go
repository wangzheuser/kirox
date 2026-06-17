package browser

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildFingerprintDataUsesConfiguredTimeZone(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)

	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -7))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	if got := decoded["timeZone"]; got != float64(-7) {
		t.Fatalf("fingerprint timeZone=%v, want -7", got)
	}
}

func TestBuildFingerprintDataIncludesEmailHashForProfileSubmit(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	email := "user@example.test"

	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", len(email), email, -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	canvas, ok := decoded["canvas"].(map[string]interface{})
	if !ok {
		t.Fatalf("canvas has type %T, want object", decoded["canvas"])
	}
	emailHash, ok := canvas["emailHash"].(float64)
	if !ok {
		t.Fatalf("canvas.emailHash has type %T, want numeric CRC-style value", canvas["emailHash"])
	}
	if emailHash == 0 {
		t.Fatalf("canvas.emailHash=%v, want non-zero numeric value", emailHash)
	}

	otherRaw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", len("other@example.test"), "other@example.test", -8))
	var otherDecoded map[string]interface{}
	if err := json.Unmarshal([]byte(otherRaw), &otherDecoded); err != nil {
		t.Fatalf("second fingerprint JSON decode failed: %v", err)
	}
	otherCanvas, ok := otherDecoded["canvas"].(map[string]interface{})
	if !ok {
		t.Fatalf("second canvas has type %T, want object", otherDecoded["canvas"])
	}
	if otherCanvas["emailHash"] != emailHash {
		t.Fatalf("canvas.emailHash changed across emails on same profiled form: first=%v other=%v", emailHash, otherCanvas["emailHash"])
	}
}

func TestBuildFingerprintDataIncludesNullDNTWhenBrowserExposesUnsetDNT(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)

	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	if got, ok := decoded["dnt"]; !ok || got != nil {
		t.Fatalf("fingerprint dnt presence/value = (%v, %v), want present null", ok, got)
	}
}

func TestBuildFingerprintDataUsesPostAuthMethodForProfileSubmit(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)

	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	auth, ok := decoded["auth"].(map[string]interface{})
	if !ok {
		t.Fatalf("auth has type %T, want object", decoded["auth"])
	}
	form, ok := auth["form"].(map[string]interface{})
	if !ok {
		t.Fatalf("auth.form has type %T, want object", auth["form"])
	}
	if got := form["method"]; got != "post" {
		t.Fatalf("auth.form.method=%v, want post", got)
	}
}

func TestBuildFingerprintDataUsesRealEmailInputNameForProfileSubmit(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	email := "user@example.test"

	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", len(email), email, -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	form, ok := decoded["form"].(map[string]interface{})
	if !ok {
		t.Fatalf("form has type %T, want object", decoded["form"])
	}
	if _, ok := form["email"]; !ok {
		t.Fatalf("form keys=%v, want key email", keysOf(form))
	}
	for key := range form {
		if strings.HasPrefix(key, "formField") {
			t.Fatalf("form contains synthetic key %q, want real email input name", key)
		}
	}
}

func TestBuildFingerprintDataProfileSubmitMatchesProfileFormBasics(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	email := "user@example.test"

	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", len(email), email, -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	for _, absent := range []string{"deviceMemory", "hardwareConcurrency", "platform"} {
		if _, ok := decoded[absent]; ok {
			t.Fatalf("profile submit fingerprint contains %s=%v, want omitted", absent, decoded[absent])
		}
	}
	token, ok := decoded["token"].(map[string]interface{})
	if !ok {
		t.Fatalf("token has type %T, want object", decoded["token"])
	}
	if got := token["isCompatible"]; got != false {
		t.Fatalf("token.isCompatible=%v, want false for profile form submit collector", got)
	}
	form := decoded["form"].(map[string]interface{})
	emailField := form["email"].(map[string]interface{})
	if got := emailField["prefilled"]; got != true {
		t.Fatalf("form.email.prefilled=%v, want true when email input already has a value", got)
	}
}

func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
