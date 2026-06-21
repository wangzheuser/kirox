package core

import "testing"

func TestBuildFinalRegistrationResultFailsWhenVerificationFailed(t *testing.T) {
	result := buildFinalRegistrationResult(&Registrar{
		Cfg:          &Config{Password: "Passw0rd!"},
		Email:        "user@example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		DeviceCode:   "device-code",
	}, map[string]interface{}{"refreshToken": "aws-refresh"}, map[string]interface{}{"refreshToken": "kiro-refresh"}, map[string]interface{}{
		"alive": false,
		"error": "models query failed: 403",
	})

	if result["status"] != "failed" {
		t.Fatalf("verification failure should fail final registration result: %#v", result)
	}
	if result["error"] != "验活失败: models query failed: 403" {
		t.Fatalf("unexpected final error: %#v", result["error"])
	}
	if passwordSet, _ := result["passwordSet"].(bool); !passwordSet {
		t.Fatalf("passwordSet should remain true after post-password verification failure: %#v", result)
	}
	if got := result["verify"]; got == nil {
		t.Fatalf("failed result should retain verify payload: %#v", result)
	}
	if result["client_id"] != "client-id" || result["client_secret"] != "client-secret" || result["device_code"] != "device-code" {
		t.Fatalf("post-password verification failure should retain OIDC credentials for re-verification: %#v", result)
	}
	if result["password"] != "Passw0rd!" || result["aws_token"] == nil || result["kiro_tokens"] == nil {
		t.Fatalf("post-password verification failure should retain account payload for salvage: %#v", result)
	}
}

func TestBuildFinalRegistrationResultSucceedsWhenVerificationAlive(t *testing.T) {
	result := buildFinalRegistrationResult(&Registrar{
		Cfg:          &Config{Password: "Passw0rd!"},
		Email:        "user@example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		DeviceCode:   "device-code",
	}, map[string]interface{}{"refreshToken": "aws-refresh"}, map[string]interface{}{"refreshToken": "kiro-refresh"}, map[string]interface{}{
		"alive": true,
		"email": "user@example.com",
	})

	if result["status"] != "success" {
		t.Fatalf("alive verification should keep final registration successful: %#v", result)
	}
	if result["client_id"] != "client-id" || result["password"] != "Passw0rd!" {
		t.Fatalf("success result lost credentials: %#v", result)
	}
}

func TestBuildFinalRegistrationResultKeepsSuspendedFailure(t *testing.T) {
	result := buildFinalRegistrationResult(&Registrar{
		Cfg:          &Config{Password: "Passw0rd!"},
		Email:        "user@example.com",
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		DeviceCode:   "device-code",
	}, map[string]interface{}{"refreshToken": "aws-refresh"}, map[string]interface{}{"refreshToken": "kiro-refresh"}, map[string]interface{}{
		"alive":     false,
		"suspended": true,
		"error":     "suspended",
	})

	if result["status"] != "failed" || result["error"] != "suspended" {
		t.Fatalf("suspended verification should remain suspended failure: %#v", result)
	}
	if got := result["verify"]; got == nil {
		t.Fatalf("suspended result should retain verify payload: %#v", result)
	}
	if result["client_id"] != "client-id" || result["password"] != "Passw0rd!" || result["aws_token"] == nil || result["kiro_tokens"] == nil {
		t.Fatalf("post-password suspended result should retain account payload for delayed re-verification: %#v", result)
	}
}

func TestBuildFinalRegistrationResultHandlesMissingRegistrar(t *testing.T) {
	result := buildFinalRegistrationResult(nil, nil, nil, map[string]interface{}{
		"alive": false,
		"error": "models query failed: 403",
	})

	if result["status"] != "failed" {
		t.Fatalf("missing registrar should still produce failed result: %#v", result)
	}
	if result["email"] != "" || result["password"] != "" {
		t.Fatalf("missing registrar should keep identity fields empty instead of panicking: %#v", result)
	}
}
