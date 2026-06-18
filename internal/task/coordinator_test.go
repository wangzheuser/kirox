package task

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"reg_go/internal/core"
	"reg_go/internal/email"
	"reg_go/internal/kirorsync"
	"reg_go/internal/storage"
)

type taskFakeTempEmailService struct {
	address string
}

func (s *taskFakeTempEmailService) Create() string {
	return s.address
}

func (s *taskFakeTempEmailService) WaitForCode(int, int) (string, error) {
	return "", nil
}

func (s *taskFakeTempEmailService) GetAddress() string {
	return s.address
}

func TestConcurrentStartStaggerDisabledForSerial(t *testing.T) {
	if got := concurrentStartStagger(0, 1); got != 0 {
		t.Fatalf("serial stagger should be 0, got %s", got)
	}
}

func TestConcurrentStartStaggerSpreadsInitialConcurrencyWindow(t *testing.T) {
	cases := []struct {
		idx      int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{idx: 0, minDelay: 0, maxDelay: concurrentStartStaggerJitterMax},
		{idx: 1, minDelay: 100 * time.Millisecond, maxDelay: 100*time.Millisecond + concurrentStartStaggerJitterMax},
		{idx: 9, minDelay: 900 * time.Millisecond, maxDelay: 900*time.Millisecond + concurrentStartStaggerJitterMax},
		{idx: 10, minDelay: 0, maxDelay: concurrentStartStaggerJitterMax},
	}

	for _, tc := range cases {
		got := concurrentStartStagger(tc.idx, 10)
		if got < tc.minDelay || got > tc.maxDelay {
			t.Fatalf("idx %d stagger = %s, want within [%s, %s]", tc.idx, got, tc.minDelay, tc.maxDelay)
		}
	}
}

func TestFilterAccountsByEmailReturnsOnlyCurrentBatch(t *testing.T) {
	accounts := []map[string]interface{}{
		{"email": "old@example.com"},
		{"email": "new@example.com"},
		{"email": "another@example.com"},
	}

	got := filterAccountsByEmail(accounts, []string{"new@example.com"})

	if len(got) != 1 || got[0]["email"] != "new@example.com" {
		t.Fatalf("expected only current batch account, got %#v", got)
	}
}

func TestSuccessfulSyncEmailsReturnsOnlySuccessfulDetails(t *testing.T) {
	got := successfulSyncEmails(kirorsync.SyncResult{Details: []kirorsync.SyncDetail{
		{Email: "ok@example.com", Success: true},
		{Email: "failed@example.com", Success: false},
	}})

	if len(got) != 1 || got[0] != "ok@example.com" {
		t.Fatalf("expected only successful email, got %#v", got)
	}
}

func TestSelectAccountsByEmailReportsMissingBatchEmails(t *testing.T) {
	accounts := []map[string]interface{}{
		{"email": "saved@example.com"},
	}

	selected, missing := selectAccountsByEmail(accounts, []string{"saved@example.com", "missing@example.com"})

	if len(selected) != 1 || selected[0]["email"] != "saved@example.com" {
		t.Fatalf("expected saved account selected, got %#v", selected)
	}
	if len(missing) != 1 || missing[0] != "missing@example.com" {
		t.Fatalf("expected missing email reported, got %#v", missing)
	}
}

// TestEffectiveSuccessTargetUsesCountWhenDisabled 验证未配置成功目标时沿用注册数量。
func TestEffectiveSuccessTargetUsesCountWhenDisabled(t *testing.T) {
	got := effectiveSuccessTarget(StartTaskRequest{Count: 7, SuccessTarget: 0})
	if got != 7 {
		t.Fatalf("effectiveSuccessTarget() = %d, want 7", got)
	}
}

// TestEffectiveSuccessTargetPrefersSuccessTarget 验证成功目标优先于注册数量。
func TestEffectiveSuccessTargetPrefersSuccessTarget(t *testing.T) {
	got := effectiveSuccessTarget(StartTaskRequest{Count: 100, SuccessTarget: 5})
	if got != 5 {
		t.Fatalf("effectiveSuccessTarget() = %d, want 5", got)
	}
}

func TestReusableEmailPoolTakesCandidateOnce(t *testing.T) {
	pool := &reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@example.com"}

	if ok := pool.put(reusableEmailCandidate{provider: "mailporary", address: service.address, tempEmailService: service}); !ok {
		t.Fatalf("expected reusable candidate to be accepted")
	}
	if _, ok := pool.take("moemail"); ok {
		t.Fatalf("different provider should not take mailporary candidate")
	}
	got, ok := pool.take("mailporary")
	if !ok || got.address != "reuse@example.com" {
		t.Fatalf("expected mailporary candidate, got %#v, ok=%v", got, ok)
	}
	if _, ok := pool.take("mailporary"); ok {
		t.Fatalf("candidate should be removed after first take")
	}
}

func TestReusableEmailDisabledByDefault(t *testing.T) {
	req := StartTaskRequest{}

	if shouldUseReusableFailedEmail(req) {
		t.Fatalf("failed email reuse should be disabled by default")
	}
}

func TestReusableEmailEnabledByRequest(t *testing.T) {
	req := StartTaskRequest{ReuseFailedEmail: true}

	if !shouldUseReusableFailedEmail(req) {
		t.Fatalf("failed email reuse should be enabled when requested")
	}
}

func TestTakeReusableFailedEmailRequiresSwitch(t *testing.T) {
	pool := &reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@example.com"}
	pool.put(reusableEmailCandidate{provider: "mailporary", address: service.address, tempEmailService: service})

	if _, ok := takeReusableFailedEmail(StartTaskRequest{}, pool, "mailporary"); ok {
		t.Fatalf("disabled failed email reuse should not take a candidate")
	}
	if len(pool.items) != 1 {
		t.Fatalf("disabled failed email reuse should keep candidate in pool, got %d", len(pool.items))
	}

	got, ok := takeReusableFailedEmail(StartTaskRequest{ReuseFailedEmail: true}, pool, "mailporary")
	if !ok || got.address != service.address {
		t.Fatalf("enabled failed email reuse should take candidate, got %#v ok=%v", got, ok)
	}
	if len(pool.items) != 0 {
		t.Fatalf("enabled failed email reuse should remove candidate from pool, got %d", len(pool.items))
	}
}

func TestReusableEmailSupportsEmailnatorProvider(t *testing.T) {
	pool := &reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@gmail.com"}
	pool.put(reusableEmailCandidate{provider: "emailnator", address: service.address, tempEmailService: service})

	candidate, ok := takeReusableFailedEmail(StartTaskRequest{ReuseFailedEmail: true}, pool, "emailnator")
	if !ok {
		t.Fatalf("expected emailnator reusable candidate")
	}
	cfg := &core.Config{}
	address, applied := applyReusableEmailCandidate("emailnator", cfg, candidate)
	if !applied || address != "reuse@gmail.com" {
		t.Fatalf("emailnator candidate not applied: address=%q applied=%v", address, applied)
	}
	if cfg.TempEmailService != service {
		t.Fatalf("emailnator should reuse TempEmailService")
	}

	extracted, ok := reusableEmailCandidateFromConfig("emailnator", cfg)
	if !ok || extracted.address != "reuse@gmail.com" || extracted.tempEmailService != service {
		t.Fatalf("emailnator candidate extraction failed: %#v ok=%v", extracted, ok)
	}
}

func TestRecycleReusableFailedEmailRequiresSwitch(t *testing.T) {
	service := &taskFakeTempEmailService{address: "reuse@example.com"}
	cfg := &core.Config{TempEmailService: service}
	result := map[string]interface{}{
		"status": "failed",
		"error":  "注册被拦截: 请更换IP或稍后重试",
	}

	disabledPool := &reusableEmailPool{}
	if _, ok := recycleReusableFailedEmail(StartTaskRequest{}, disabledPool, "mailporary", cfg, result, false); ok {
		t.Fatalf("disabled failed email reuse should not recycle candidate")
	}
	if len(disabledPool.items) != 0 {
		t.Fatalf("disabled failed email reuse should not write to pool, got %d", len(disabledPool.items))
	}

	enabledPool := &reusableEmailPool{}
	candidate, ok := recycleReusableFailedEmail(StartTaskRequest{ReuseFailedEmail: true}, enabledPool, "mailporary", cfg, result, false)
	if !ok || candidate.address != service.address {
		t.Fatalf("enabled failed email reuse should recycle candidate, got %#v ok=%v", candidate, ok)
	}
	if len(enabledPool.items) != 1 {
		t.Fatalf("enabled failed email reuse should write one candidate, got %d", len(enabledPool.items))
	}
}

func TestIsReusableEmailError(t *testing.T) {
	reusable := []string{
		"注册被拦截: 请更换IP或稍后重试",
		"注册失败: IP或浏览器指纹被检测，请更换代理或重新生成指纹",
		"send-otp 失败 (400): BLOCKED",
		"网络连接超时，请检查网络或代理设置",
	}
	for _, errorMsg := range reusable {
		if !isReusableEmailError(errorMsg) {
			t.Fatalf("expected reusable error: %s", errorMsg)
		}
	}

	notReusable := []string{
		"邮箱已注册过，跳过",
		"验证码错误: 验证码无效或已过期，请重试",
		"验证码接收超时，请检查邮箱服务或稍后重试",
		"邮箱服务异常，无法接收验证码",
		"账号已被封禁",
	}
	for _, errorMsg := range notReusable {
		if isReusableEmailError(errorMsg) {
			t.Fatalf("expected non-reusable error: %s", errorMsg)
		}
	}
}

func TestShouldRecycleReusableEmailRejectsPasswordSet(t *testing.T) {
	result := map[string]interface{}{
		"status":      "failed",
		"error":       "注册被拦截: 请更换IP或稍后重试",
		"passwordSet": true,
	}

	if shouldRecycleReusableEmail(result) {
		t.Fatalf("passwordSet=true should not recycle reusable email")
	}
}

func TestEmailnatorSendOTP400DoesNotTriggerKillSwitch(t *testing.T) {
	if isKillSwitchError("send-otp 失败 (400): domain rejected", "emailnator") {
		t.Fatalf("emailnator send-otp 400 should be treated as single mailbox failure")
	}
	if !isKillSwitchError("注册请求被拦截 BLOCKED", "emailnator") {
		t.Fatalf("emailnator BLOCKED/IP risk should still trigger kill switch")
	}
}

func TestMailGWDoesNotRequireOutlookAccounts(t *testing.T) {
	Manager.mu.Lock()
	Manager.running = false
	Manager.mu.Unlock()

	result := StartTask(StartTaskRequest{Count: 1, EmailProvider: "mailgw"})
	if errText, _ := result["error"].(string); strings.Contains(errText, "微软邮箱") || strings.Contains(errText, "Outlook") {
		t.Fatalf("mailgw should not require Outlook accounts, got error %q", errText)
	}
	StopTask(true)
}

func TestPrepareMoeMailStartRequestBackfillsMissingConfigsFromSavedList(t *testing.T) {
	req := StartTaskRequest{
		EmailProvider:  "moemail",
		MoeMailDomains: []string{"wqpnode.filegear-sg.me"},
	}
	calls := 0

	got := prepareMoeMailStartRequest(req, func() []email.MoeMailConfig {
		calls++
		return []email.MoeMailConfig{{
			Name:   "saved",
			URL:    "https://moemail.example",
			APIKey: "key",
		}}
	})

	if calls != 1 {
		t.Fatalf("expected saved MoeMail configs to be loaded once, got %d calls", calls)
	}
	configs := got.MoeMailConfigs["wqpnode.filegear-sg.me"]
	if len(configs) != 1 || configs[0].Name != "saved" {
		t.Fatalf("expected selected domain to be backed by saved config, got %#v", got.MoeMailConfigs)
	}
}

func TestValidateMoeMailDeliverabilityRejectsDomainWithoutMX(t *testing.T) {
	err := validateMoeMailDeliverability(StartTaskRequest{
		EmailProvider:  "moemail",
		MoeMailDomains: []string{"wqpnode.filegear-sg.me"},
	}, func(domain string) (bool, error) {
		if domain != "wqpnode.filegear-sg.me" {
			t.Fatalf("unexpected MX lookup domain %q", domain)
		}
		return false, nil
	})
	if err == nil || !strings.Contains(err.Error(), "缺少 MX") {
		t.Fatalf("domain without MX should be rejected with clear diagnostic, got %v", err)
	}
}

func TestValidateMoeMailDeliverabilityAllowsDomainWithMX(t *testing.T) {
	err := validateMoeMailDeliverability(StartTaskRequest{
		EmailProvider:  "moemail",
		MoeMailDomains: []string{"codeai.de5.net"},
	}, func(domain string) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("domain with MX should be allowed: %v", err)
	}
}

func TestReusableEmailSupportsMailGWProvider(t *testing.T) {
	pool := reusableEmailPool{}
	service := &taskFakeTempEmailService{address: "reuse@oakon.com"}
	pool.put(reusableEmailCandidate{provider: "mailgw", address: service.address, tempEmailService: service})

	candidate, ok := pool.take("mailgw")
	if !ok {
		t.Fatalf("expected mailgw reusable candidate")
	}
	var cfg core.Config
	address, applied := applyReusableEmailCandidate("mailgw", &cfg, candidate)
	if !applied || address != service.address {
		t.Fatalf("mailgw reusable candidate not applied, address=%q applied=%v", address, applied)
	}
	if cfg.TempEmailService != service {
		t.Fatalf("mailgw should reuse TempEmailService")
	}
}

func TestMailGWSendOTP400DoesNotTriggerKillSwitch(t *testing.T) {
	if isKillSwitchError("send-otp 失败 (400): domain rejected", "mailgw") {
		t.Fatalf("mailgw send-otp 400 should be treated as single mailbox failure")
	}
	if !isKillSwitchError("注册请求被拦截 BLOCKED", "mailgw") {
		t.Fatalf("mailgw BLOCKED/IP risk should still trigger kill switch")
	}
}

func TestTemporaryEmailSendOTP400BlockedTriggersKillSwitch(t *testing.T) {
	cases := []string{
		`send-otp 失败 (400): {"errorCode":"BLOCKED","message":"Request was blocked by TES."}`,
		`send-otp 失败 (400): Request was blocked by TES.`,
	}
	for _, errText := range cases {
		if !isKillSwitchError(errText, "tempmail_lol") {
			t.Fatalf("temporary email send-otp BLOCKED/TES should trigger kill switch: %s", errText)
		}
	}
}

func TestClassifySendOTP400DomainRejected(t *testing.T) {
	got := classifyError("send-otp 失败 (400): domain rejected")
	if got != "验证码发送失败" {
		t.Fatalf("plain send-otp 400 should be classified as OTP send failure, got %q", got)
	}
}

func TestPlainSendOTP400IsNotReusableEmailError(t *testing.T) {
	if isReusableEmailError("send-otp 失败 (400): domain rejected") {
		t.Fatalf("plain send-otp 400 should not reuse the same temporary mailbox")
	}
}

func TestShouldForceStopTaskIgnoresKillSwitchForTESBlocked(t *testing.T) {
	errText := `send-otp 失败 (400): {"errorCode":"BLOCKED","message":"Request was blocked by TES."}`
	if !shouldForceStopTask(errText, "outlook", false) {
		t.Fatalf("outlook TES/BLOCKED should force-stop even when kill switch setting is disabled")
	}
}

func TestSendOTPBlockedFriendlyErrorForcesStopForOutlookEvenWhenKillSwitchDisabled(t *testing.T) {
	errText := `注册被拦截: 请更换IP或稍后重试 [provider=outlook, domain=example.com, emailProxy=enabled, proxy=enabled]`
	if !shouldForceStopTask(errText, "outlook", false) {
		t.Fatalf("formatted send-otp blocked error should force-stop for outlook even when kill switch setting is disabled")
	}
}

func TestSendOTPBlockedFriendlyErrorIsNotProxyNetworkError(t *testing.T) {
	errText := `注册被拦截: 请更换IP或稍后重试 [provider=tempmail_lol, domain=example.com, emailProxy=enabled, proxy=enabled]`
	if isProxyNetworkError(errText) {
		t.Fatalf("send-otp blocked diagnostics include proxy=enabled but should not be treated as proxy network error")
	}
}

func TestShouldForceStopTaskRespectsKillSwitchForPlainSendOTP400(t *testing.T) {
	if shouldForceStopTask("send-otp 失败 (400): domain rejected", "tempmail_lol", false) {
		t.Fatalf("plain temporary-email send-otp 400 should not force-stop")
	}
	if shouldForceStopTask("send-otp 失败 (400): domain rejected", "outlook", false) {
		t.Fatalf("plain send-otp 400 should not force-stop when kill switch setting is disabled")
	}
	if !shouldForceStopTask("send-otp 失败 (400): domain rejected", "outlook", true) {
		t.Fatalf("outlook send-otp 400 should still honor enabled kill switch")
	}
}

func TestTemporaryEmailSendOTPBlockedDoesNotForceStopWholeBatch(t *testing.T) {
	errText := `注册被拦截: 请更换IP或稍后重试 [provider=tempmail_lol, domain=example.com, emailProxy=enabled, proxy=enabled]`
	if shouldForceStopTask(errText, "tempmail_lol", false) {
		t.Fatalf("temporary-email TES/BLOCKED should fail the current mailbox but not force-stop the whole batch")
	}
}

func TestOutlookSendOTPBlockedStillForceStopsWholeBatch(t *testing.T) {
	errText := `注册被拦截: 请更换IP或稍后重试 [provider=outlook, domain=example.com, emailProxy=enabled, proxy=enabled]`
	if !shouldForceStopTask(errText, "outlook", false) {
		t.Fatalf("outlook TES/BLOCKED should still force-stop because accounts are not throwaway domains")
	}
}

func TestSuccessTargetModeDoesNotForceStopOnPerAttemptRiskErrors(t *testing.T) {
	cases := []struct {
		name       string
		errorMsg   string
		provider   string
		killSwitch bool
	}{
		{
			name:     "outlook formatted blocked",
			errorMsg: `注册被拦截: 请更换IP或稍后重试 [provider=outlook, domain=example.com, emailProxy=enabled, proxy=enabled]`,
			provider: "outlook",
		},
		{
			name:     "outlook TES blocked",
			errorMsg: `send-otp 失败 (400): {"errorCode":"BLOCKED","message":"Request was blocked by TES."}`,
			provider: "outlook",
		},
		{
			name:       "plain send otp 400 with kill switch",
			errorMsg:   `send-otp 失败 (400): domain rejected`,
			provider:   "outlook",
			killSwitch: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if shouldForceStopTaskForMode(tc.errorMsg, tc.provider, tc.killSwitch, true) {
				t.Fatalf("success target mode should continue after per-attempt risk error: %s", tc.errorMsg)
			}
			if !shouldForceStopTaskForMode(tc.errorMsg, tc.provider, tc.killSwitch, false) {
				t.Fatalf("non-success-target mode should keep existing force-stop behavior: %s", tc.errorMsg)
			}
		})
	}
}

func TestSuccessTargetModeDoesNotStopOnOutlookOTPTimeoutStreak(t *testing.T) {
	var streak outlookOTPTimeoutStreak
	for i := 0; i < 5; i++ {
		if shouldStopForOutlookOTPTimeout(true, false, "验证码超时", true, &streak) {
			t.Fatalf("success target mode should not stop on OTP timeout streak at attempt %d", i+1)
		}
	}

	if streak.count != 5 {
		t.Fatalf("success target mode should still record timeout streak for diagnostics, got %d", streak.count)
	}
}

func TestNormalModeStopsOnOutlookOTPTimeoutStreak(t *testing.T) {
	var streak outlookOTPTimeoutStreak
	for i := 0; i < 4; i++ {
		if shouldStopForOutlookOTPTimeout(true, false, "验证码超时", false, &streak) {
			t.Fatalf("normal mode should not stop before fifth timeout, stopped at %d", i+1)
		}
	}
	if !shouldStopForOutlookOTPTimeout(true, false, "验证码超时", false, &streak) {
		t.Fatalf("normal mode should stop on fifth consecutive OTP timeout")
	}
}

func TestTemporaryEmailSendOTPBlockedDoesNotRetrySameMailbox(t *testing.T) {
	errText := `注册被拦截: 请更换IP或稍后重试 [provider=tempmail_lol, domain=example.com, emailProxy=enabled, proxy=enabled]`
	if shouldRetrySameMailboxAfterFailure(errText, "tempmail_lol") {
		t.Fatalf("temporary-email TES/BLOCKED should move to next mailbox instead of retrying same rejected domain")
	}
}

func TestPlainTemporaryEmailErrorCanRetrySameMailbox(t *testing.T) {
	if !shouldRetrySameMailboxAfterFailure("验证码接收超时，请检查邮箱服务或稍后重试", "tempmail_lol") {
		t.Fatalf("non-send-otp-blocked temporary email errors should keep existing retry behavior")
	}
}

func TestTemporaryEmailSendOTPBlockedDoesNotRecycleReusableMailbox(t *testing.T) {
	pool := &reusableEmailPool{}
	cfg := &core.Config{TempEmailService: &taskFakeTempEmailService{address: "blocked@example.test"}}
	result := map[string]interface{}{
		"status": "failed",
		"error":  `注册被拦截: 请更换IP或稍后重试 [provider=tempmail_lol, domain=example.test, emailProxy=enabled, proxy=enabled]`,
	}

	if _, ok := recycleReusableFailedEmail(StartTaskRequest{ReuseFailedEmail: true}, pool, "tempmail_lol", cfg, result, false); ok {
		t.Fatalf("temporary-email TES/BLOCKED should not recycle the same rejected mailbox")
	}
	if _, ok := pool.take("tempmail_lol"); ok {
		t.Fatalf("temporary-email TES/BLOCKED mailbox should not be present in reusable pool")
	}
}

func TestClassifyOutlookGraphInvalidGrantAsAbnormalMailbox(t *testing.T) {
	errText := `刷新 Outlook Graph Token 失败: 刷新失败 400: {"error":"invalid_grant","error_description":"AADSTS70000: User account is found to be in service abuse mode."}`
	if got := classifyError(errText); got != "异常邮箱" {
		t.Fatalf("Outlook Graph invalid_grant should classify as abnormal mailbox, got %q", got)
	}
}

func TestShouldRotateOutlookAccountAfterGraphInvalidGrant(t *testing.T) {
	errText := `刷新 Outlook Graph Token 失败: 刷新失败 400: {"error":"invalid_grant","error_description":"AADSTS70000: User account is found to be in service abuse mode."}`
	if !shouldRotateOutlookAccountAfterFailure(errText) {
		t.Fatalf("Outlook Graph invalid_grant/service abuse should rotate to the next Outlook account")
	}
}

func TestOutlookAccountRotationResetsProxySwitchBudget(t *testing.T) {
	attempt, proxySwitches := resetOutlookRetryBudgetAfterAccountRotation(2, 3)
	if attempt != -1 {
		t.Fatalf("account rotation should restart retry attempts from zero on next loop, got attempt=%d", attempt)
	}
	if proxySwitches != 0 {
		t.Fatalf("account rotation should reset proxy switch budget for the new mailbox, got %d", proxySwitches)
	}
}

func TestBuildAvailableOutlookAccountsDefersPreviousSendOTPBlockedAccounts(t *testing.T) {
	stored := []map[string]interface{}{
		{"email": "blocked@outlook.de", "password": "p1", "clientId": "c1", "refreshToken": "r1", "registered": false, "failReason": "IP/指纹风控"},
		{"email": "timeout@outlook.jp", "password": "p0", "clientId": "c0", "refreshToken": "r0", "registered": false, "failReason": "验证码超时"},
		{"email": "clean@outlook.jp", "password": "p2", "clientId": "c2", "refreshToken": "r2", "registered": false},
		{"email": "registered@outlook.jp", "password": "p3", "clientId": "c3", "refreshToken": "r3", "registered": true},
	}

	got := buildAvailableOutlookAccounts(stored)
	if len(got) != 3 {
		t.Fatalf("expected 3 unregistered Outlook accounts, got %#v", got)
	}
	if got[0].Email != "clean@outlook.jp" || got[1].Email != "blocked@outlook.de" || got[2].Email != "timeout@outlook.jp" {
		t.Fatalf("previous send-otp/OTP-problem accounts should be deferred after clean accounts, got %#v", got)
	}
}

func TestBuildAvailableOutlookAccountsPreservesRegistrationEmail(t *testing.T) {
	stored := []map[string]interface{}{
		{"email": "alias@outlook.jp", "password": "p", "clientId": "c", "refreshToken": "r", "registered": false, "registrationEmail": "actual@hotmail.com"},
	}

	got := buildAvailableOutlookAccounts(stored)
	if len(got) != 1 {
		t.Fatalf("expected one account, got %#v", got)
	}
	if got[0].RegistrationEmail != "actual@hotmail.com" {
		t.Fatalf("RegistrationEmail should be preserved from storage, got %#v", got[0])
	}
}

func TestResolveOutlookGraphRegistrationEmailMapsOnlyClaimedAccount(t *testing.T) {
	accounts := []email.OutlookAccount{
		{Email: "first@outlook.jp", ClientID: "c1", RefreshToken: "r1"},
		{Email: "second@outlook.jp", ClientID: "c2", RefreshToken: "r2"},
	}
	calls := 0
	resolver := func(acc email.OutlookAccount, proxyURL string) (string, error) {
		calls++
		if acc.Email != "first@outlook.jp" {
			t.Fatalf("should only resolve claimed account, got %s", acc.Email)
		}
		return "first@hotmail.com", nil
	}

	resolved := resolveOutlookGraphRegistrationEmail(accounts[0], "", resolver)

	if calls != 1 {
		t.Fatalf("expected one lazy Graph /me lookup for the claimed account, got %d", calls)
	}
	if resolved.RegistrationEmail != "first@hotmail.com" {
		t.Fatalf("RegistrationEmail not set from Graph /me: %#v", resolved)
	}
	if accounts[1].RegistrationEmail != "" {
		t.Fatalf("unclaimed account must not be pre-resolved: %#v", accounts[1])
	}
}

func TestResolveOutlookGraphRegistrationEmailAutoKeepsExistingMappingWithoutLookup(t *testing.T) {
	acc := email.OutlookAccount{Email: "alias@outlook.jp", RegistrationEmail: "actual@hotmail.com"}
	calls := 0
	resolved := resolveOutlookGraphRegistrationEmailWithMode(acc, "", storage.OutlookGraphRegistrationEmailAuto, func(email.OutlookAccount, string) (email.OutlookGraphProfile, error) {
		calls++
		return email.OutlookGraphProfile{PrimaryEmail: "unexpected@hotmail.com"}, nil
	})

	if calls != 0 {
		t.Fatalf("auto mode should reuse existing RegistrationEmail without Graph /me lookup, got %d calls", calls)
	}
	if resolved.RegistrationEmail != "actual@hotmail.com" {
		t.Fatalf("auto mode should keep existing mapping: %#v", resolved)
	}
}

func TestResolveOutlookGraphRegistrationEmailAutoPrefersImportedAliasWhenGraphKnowsIt(t *testing.T) {
	acc := email.OutlookAccount{Email: "alias@outlook.jp"}
	profile := email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com", Aliases: []string{"primary@hotmail.com", "alias@outlook.jp"}, AliasDataAvailable: true}

	resolved := resolveOutlookGraphRegistrationEmailWithMode(acc, "", storage.OutlookGraphRegistrationEmailAuto, func(email.OutlookAccount, string) (email.OutlookGraphProfile, error) {
		return profile, nil
	})

	if resolved.RegistrationEmail != "alias@outlook.jp" {
		t.Fatalf("auto mode should keep imported alias when Graph lists it: %#v", resolved)
	}
	if resolved.GraphPrimaryEmail != "primary@hotmail.com" {
		t.Fatalf("Graph primary email should be recorded: %#v", resolved)
	}
}

func TestResolveOutlookGraphRegistrationEmailAutoUsesPrimaryWhenImportedAddressIsNotAlias(t *testing.T) {
	acc := email.OutlookAccount{Email: "external@example.com"}
	profile := email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com", Aliases: []string{"primary@hotmail.com", "alias@outlook.jp"}, AliasDataAvailable: true}

	resolved := resolveOutlookGraphRegistrationEmailWithMode(acc, "", storage.OutlookGraphRegistrationEmailAuto, func(email.OutlookAccount, string) (email.OutlookGraphProfile, error) {
		return profile, nil
	})

	if resolved.RegistrationEmail != "primary@hotmail.com" {
		t.Fatalf("auto mode should use primary when Graph alias list excludes imported address: %#v", resolved)
	}
}

func TestResolveOutlookGraphRegistrationEmailAutoKeepsImportedWhenAliasesUnavailable(t *testing.T) {
	acc := email.OutlookAccount{Email: "alias@outlook.jp"}
	profile := email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com"}

	resolved := resolveOutlookGraphRegistrationEmailWithMode(acc, "", storage.OutlookGraphRegistrationEmailAuto, func(email.OutlookAccount, string) (email.OutlookGraphProfile, error) {
		return profile, nil
	})

	if resolved.RegistrationEmail != "alias@outlook.jp" {
		t.Fatalf("auto mode should keep imported address when aliases are unavailable: %#v", resolved)
	}
}

func TestResolveOutlookGraphRegistrationEmailPrimaryModeOverridesExistingMapping(t *testing.T) {
	acc := email.OutlookAccount{Email: "alias@outlook.jp", RegistrationEmail: "old-alias@hotmail.com"}
	calls := 0
	resolved := resolveOutlookGraphRegistrationEmailWithMode(acc, "", storage.OutlookGraphRegistrationEmailPrimary, func(email.OutlookAccount, string) (email.OutlookGraphProfile, error) {
		calls++
		return email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com", Aliases: []string{"alias@outlook.jp"}, AliasDataAvailable: true}, nil
	})

	if calls != 1 {
		t.Fatalf("primary mode should refresh Graph /me even when RegistrationEmail exists, got %d calls", calls)
	}
	if resolved.RegistrationEmail != "primary@hotmail.com" {
		t.Fatalf("primary mode should override existing mapping with Graph primary: %#v", resolved)
	}
}

func TestResolveOutlookGraphRegistrationEmailImportedModeOverridesExistingMapping(t *testing.T) {
	acc := email.OutlookAccount{Email: "alias@outlook.jp", RegistrationEmail: "old-primary@hotmail.com"}
	calls := 0
	resolved := resolveOutlookGraphRegistrationEmailWithMode(acc, "", storage.OutlookGraphRegistrationEmailImported, func(email.OutlookAccount, string) (email.OutlookGraphProfile, error) {
		calls++
		return email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com"}, nil
	})

	if calls != 0 {
		t.Fatalf("imported mode should not call Graph /me, got %d calls", calls)
	}
	if resolved.RegistrationEmail != "alias@outlook.jp" {
		t.Fatalf("imported mode should override existing mapping with imported email: %#v", resolved)
	}
}

func TestResolveOutlookGraphRegistrationEmailPrimaryModeUsesGraphPrimary(t *testing.T) {
	acc := email.OutlookAccount{Email: "alias@outlook.jp"}
	profile := email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com", Aliases: []string{"alias@outlook.jp"}, AliasDataAvailable: true}

	resolved := resolveOutlookGraphRegistrationEmailWithMode(acc, "", storage.OutlookGraphRegistrationEmailPrimary, func(email.OutlookAccount, string) (email.OutlookGraphProfile, error) {
		return profile, nil
	})

	if resolved.RegistrationEmail != "primary@hotmail.com" {
		t.Fatalf("primary mode should force Graph primary: %#v", resolved)
	}
}

func TestResolveOutlookGraphRegistrationEmailImportedModeSkipsGraphLookup(t *testing.T) {
	acc := email.OutlookAccount{Email: "alias@outlook.jp"}
	calls := 0
	resolved := resolveOutlookGraphRegistrationEmailWithMode(acc, "", storage.OutlookGraphRegistrationEmailImported, func(email.OutlookAccount, string) (email.OutlookGraphProfile, error) {
		calls++
		return email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com"}, nil
	})

	if calls != 0 {
		t.Fatalf("imported mode should not call Graph /me, got %d calls", calls)
	}
	if resolved.RegistrationEmail != "alias@outlook.jp" {
		t.Fatalf("imported mode should use imported email: %#v", resolved)
	}
}

func TestBuildAvailableOutlookAccountsPreservesGraphCacheFields(t *testing.T) {
	stored := []map[string]interface{}{
		{"email": "alias@outlook.jp", "password": "p", "clientId": "c", "refreshToken": "r", "registered": false, "registrationEmail": "alias@outlook.jp", "graphPrimaryEmail": "primary@hotmail.com", "graphAliasVerified": true, "graphResolvedAt": "2026-06-18 10:00:00"},
	}

	got := buildAvailableOutlookAccounts(stored)
	if len(got) != 1 {
		t.Fatalf("expected one account, got %#v", got)
	}
	if got[0].GraphPrimaryEmail != "primary@hotmail.com" || !got[0].GraphAliasVerified || got[0].GraphResolvedAt == "" {
		t.Fatalf("Graph cache fields should be preserved from storage, got %#v", got[0])
	}
}

func TestCachedOutlookGraphProfileResolverReusesProfileByCredential(t *testing.T) {
	calls := 0
	resolver := newCachedOutlookGraphProfileResolver(func(acc email.OutlookAccount, proxyURL string) (email.OutlookGraphProfile, error) {
		calls++
		return email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com", Aliases: []string{"alias1@outlook.jp", "alias2@outlook.jp"}, AliasDataAvailable: true}, nil
	})

	for _, addr := range []string{"alias1@outlook.jp", "alias2@outlook.jp"} {
		if _, err := resolver(email.OutlookAccount{Email: addr, ClientID: "client", RefreshToken: "refresh"}, ""); err != nil {
			t.Fatalf("resolver(%s): %v", addr, err)
		}
	}
	if calls != 1 {
		t.Fatalf("same credential should use one Graph lookup, got %d", calls)
	}
}

func TestOutlookOTPTimeoutStreakTripsAtFiveAndResets(t *testing.T) {
	var streak outlookOTPTimeoutStreak
	for i := 0; i < 4; i++ {
		if streak.Record("验证码超时") {
			t.Fatalf("should not trip before fifth timeout")
		}
	}
	streak.Record("网络/代理问题")
	for i := 0; i < 4; i++ {
		if streak.Record("验证码超时") {
			t.Fatalf("non-timeout should reset streak")
		}
	}
	if !streak.Record("验证码超时") {
		t.Fatalf("fifth consecutive timeout should trip")
	}
}

func TestCachedOutlookGraphProfileResolverDoesNotCacheErrors(t *testing.T) {
	calls := 0
	resolver := newCachedOutlookGraphProfileResolver(func(acc email.OutlookAccount, proxyURL string) (email.OutlookGraphProfile, error) {
		calls++
		if calls == 1 {
			return email.OutlookGraphProfile{}, fmt.Errorf("temporary EOF")
		}
		return email.OutlookGraphProfile{PrimaryEmail: "primary@hotmail.com"}, nil
	})

	_, firstErr := resolver(email.OutlookAccount{Email: "alias1@outlook.jp", ClientID: "client", RefreshToken: "refresh"}, "")
	profile, secondErr := resolver(email.OutlookAccount{Email: "alias2@outlook.jp", ClientID: "client", RefreshToken: "refresh"}, "")
	if firstErr == nil || secondErr != nil {
		t.Fatalf("first call should fail and second should retry successfully, first=%v second=%v", firstErr, secondErr)
	}
	if calls != 2 || profile.PrimaryEmail != "primary@hotmail.com" {
		t.Fatalf("resolver should not cache errors, calls=%d profile=%+v", calls, profile)
	}
}
