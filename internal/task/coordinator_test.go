package task

import (
	"testing"
	"time"

	"reg_go/internal/core"
	"reg_go/internal/kirorsync"
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
