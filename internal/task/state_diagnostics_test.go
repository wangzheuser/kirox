package task

import "testing"

func TestRuntimeTaskStatsBuildsDiagnosticBreakdowns(t *testing.T) {
	stats := newRuntimeTaskStats()

	stats.RecordFailure(`send-otp 失败 (400): blocked [provider=mailtm, domain=example.test, emailProxy=http://mail-proxy, proxy=http://reg-proxy]`, false, "发送验证码")
	stats.RecordFailure("INVALID_OTP 验证码无效", false, "")
	stats.RecordFailure("等待验证码超时", false, "等待验证码")
	stats.RecordFailure("验活失败: models query failed: 403 forbidden", true, "验活")
	stats.RecordFailure("suspended", true, "验活")
	stats.RecordNetworkError("send-otp 请求 timeout")
	stats.RecordNetworkError("等待验证码 获取邮件失败")
	stats.RecordNetworkError("验活失败: connection reset")
	stats.RecordGraphFailure("Graph Token失效")
	stats.clashNetworkErrors = 2
	stats.clashRiskFailures = 1
	stats.poolNetworkErrors = 3

	diagnostics := stats.DiagnosticsSnapshot(4, 5, 10)

	assertGroupTotalEqualsDetails(t, "otp", diagnostics.OTPFailures)
	if diagnostics.OTPFailures.Total != 3 {
		t.Fatalf("OTP total = %d, want 3", diagnostics.OTPFailures.Total)
	}
	if diagnostics.OTPFailures.Details["验证码发送失败"] != 1 ||
		diagnostics.OTPFailures.Details["验证码无效"] != 1 ||
		diagnostics.OTPFailures.Details["验证码超时"] != 1 {
		t.Fatalf("unexpected OTP details: %#v", diagnostics.OTPFailures.Details)
	}

	assertGroupTotalEqualsDetails(t, "post-registration", diagnostics.PostRegistrationFailures)
	if diagnostics.PostRegistrationFailures.Details["模型列表失败"] != 1 ||
		diagnostics.PostRegistrationFailures.Details["账号封禁"] != 1 {
		t.Fatalf("unexpected post-registration details: %#v", diagnostics.PostRegistrationFailures.Details)
	}

	assertGroupTotalEqualsDetails(t, "network/proxy", diagnostics.NetworkProxyFailures)
	if diagnostics.NetworkProxyFailures.Details["发送验证码"] != 1 ||
		diagnostics.NetworkProxyFailures.Details["等待验证码"] != 1 ||
		diagnostics.NetworkProxyFailures.Details["验活"] != 1 {
		t.Fatalf("unexpected network stage details: %#v", diagnostics.NetworkProxyFailures.Details)
	}

	assertGroupTotalEqualsDetails(t, "graph", diagnostics.GraphFailures)
	if diagnostics.GraphFailures.Details["Graph Token失效"] != 1 {
		t.Fatalf("unexpected graph details: %#v", diagnostics.GraphFailures.Details)
	}

	assertGroupTotalEqualsDetails(t, "proxy", diagnostics.ProxyFailures)
	for name, want := range map[string]int{
		"Clash网络错误": 2,
		"Clash风控错误": 1,
		"Clash临时拉黑": 4,
		"代理池网络错误":   3,
		"代理池临时拉黑":   5,
	} {
		if got := diagnostics.ProxyFailures.Details[name]; got != want {
			t.Fatalf("proxy detail %s = %d, want %d; all=%#v", name, got, want, diagnostics.ProxyFailures.Details)
		}
	}

	if got := diagnostics.SendOTPDiagnostics["provider"][0]; got.Label != "mailtm" || got.Count != 1 {
		t.Fatalf("unexpected send-otp provider top: %#v", diagnostics.SendOTPDiagnostics["provider"])
	}
	if len(diagnostics.TopFailures) == 0 {
		t.Fatalf("expected top failure samples")
	}
}

func TestStateDiagnosticsAreResetAndCloned(t *testing.T) {
	state := &State{}
	state.SetDiagnostics(TaskDiagnostics{
		OTPFailures: DiagnosticGroup{
			Total:   1,
			Details: map[string]int{"验证码无效": 1},
		},
	})

	status := state.GetStatus()
	diagnostics, ok := status["diagnostics"].(TaskDiagnostics)
	if !ok {
		t.Fatalf("diagnostics type = %T, want TaskDiagnostics", status["diagnostics"])
	}
	diagnostics.OTPFailures.Details["验证码无效"] = 99

	status = state.GetStatus()
	diagnostics = status["diagnostics"].(TaskDiagnostics)
	if got := diagnostics.OTPFailures.Details["验证码无效"]; got != 1 {
		t.Fatalf("diagnostics map was not cloned, got %d", got)
	}

	state.ResetDiagnostics()
	status = state.GetStatus()
	diagnostics = status["diagnostics"].(TaskDiagnostics)
	if diagnostics.OTPFailures.Total != 0 || len(diagnostics.OTPFailures.Details) != 0 {
		t.Fatalf("diagnostics not reset: %#v", diagnostics.OTPFailures)
	}
}

func assertGroupTotalEqualsDetails(t *testing.T, name string, group DiagnosticGroup) {
	t.Helper()
	total := 0
	for _, count := range group.Details {
		total += count
	}
	if group.Total != total {
		t.Fatalf("%s total = %d, detail sum = %d (%#v)", name, group.Total, total, group.Details)
	}
}
