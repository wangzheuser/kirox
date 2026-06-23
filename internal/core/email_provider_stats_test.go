package core

import "testing"

func TestBasePostPasswordRegistrationResultIncludesOTPReceived(t *testing.T) {
	r := &Registrar{Email: "ok@example.com", Cfg: &Config{Password: "secret"}, OTPReceived: true}

	result := basePostPasswordRegistrationResult(r, nil, nil, nil)

	if got, _ := result["otpReceived"].(bool); !got {
		t.Fatalf("otpReceived should be true after Step10 succeeds, result=%#v", result)
	}
}
