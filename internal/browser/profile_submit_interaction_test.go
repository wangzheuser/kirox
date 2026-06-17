package browser

import (
	"encoding/json"
	"testing"
)

func TestProfileSubmitInteractionMatchesSingleEmailFieldAction(t *testing.T) {
	for i := 0; i < 20; i++ {
		identity := RandomIdentity()
		ctx := NewFPContext(identity)
		raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/?workflowID=wf#/signup/enter-email", "https://signin.aws.amazon.com/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
			t.Fatalf("fingerprint JSON decode failed: %v", err)
		}
		interaction := decoded["interaction"].(map[string]interface{})
		if got := interaction["clicks"]; got != float64(1) {
			t.Fatalf("interaction.clicks=%v, want 1", got)
		}
		if got := interaction["keyPresses"]; got != float64(1) {
			t.Fatalf("interaction.keyPresses=%v, want 1", got)
		}
		if got := len(interaction["keyPressTimeIntervals"].([]interface{})); got != 0 {
			t.Fatalf("interaction.keyPressTimeIntervals len=%d, want 0", got)
		}
		if got := len(interaction["mouseCycles"].([]interface{})); got != 0 {
			t.Fatalf("interaction.mouseCycles len=%d, want 0", got)
		}
		if got := len(interaction["mouseClickPositions"].([]interface{})); got != 1 {
			t.Fatalf("interaction.mouseClickPositions len=%d, want 1", got)
		}

		form := decoded["form"].(map[string]interface{})
		emailField := form["email"].(map[string]interface{})
		if got := emailField["keyPresses"]; got != float64(1) {
			t.Fatalf("form.email.keyPresses=%v, want 1", got)
		}
		if got := len(emailField["keyPressTimeIntervals"].([]interface{})); got != 0 {
			t.Fatalf("form.email.keyPressTimeIntervals len=%d, want 0", got)
		}
		if got := len(emailField["mouseCycles"].([]interface{})); got != 0 {
			t.Fatalf("form.email.mouseCycles len=%d, want 0", got)
		}
		if got := emailField["width"]; got != float64(188) {
			t.Fatalf("form.email.width=%v, want captured email input width 188", got)
		}
		if got := emailField["height"]; got != float64(38) {
			t.Fatalf("form.email.height=%v, want captured email input height 38", got)
		}
		focus, ok := emailField["totalFocusTime"].(float64)
		if !ok || focus < 900 || focus > 6000 {
			t.Fatalf("form.email.totalFocusTime=%v, want plausible focused dwell time", emailField["totalFocusTime"])
		}
	}
}
