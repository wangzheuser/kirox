package task

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"reg_go/internal/core"
	"reg_go/internal/data"
	"reg_go/internal/email"
	"reg_go/internal/kirorsync"
	"reg_go/internal/proxy"
	"reg_go/internal/storage"
)

const (
	concurrentStartStaggerStep         = 100 * time.Millisecond
	concurrentStartStaggerJitterMax    = 80 * time.Millisecond
	registrarStartPacingStep           = 15 * time.Second
	runtimeProxyCountryStartPacingStep = 90 * time.Second
	postPasswordRegionCooldownAfter    = 2
)

var emailProviderRateLimitBackoffs = []time.Duration{60 * time.Second, 120 * time.Second, 300 * time.Second}

type runtimeTaskStats struct {
	failCategories                map[string]int
	failureSamples                map[string]int
	preflightFailureCategories    map[string]int
	networkStageFailures          map[string]int
	graphFailureCategories        map[string]int
	passwordSetFailureCategories  map[string]int
	emailServiceFailureDetails    map[string]int
	riskFailureDetails            map[string]int
	sendOTPDiagnosticCounts       map[string]map[string]int
	registeredSkipCount           int
	graphResolveFailures          int
	clashNetworkErrors            int
	clashRiskFailures             int
	poolNetworkErrors             int
	passwordSetFailures           int
	sendOTPBlockedFailures        int
	preflightEmailCreateFailures  int
	preflightProxySelectFailures  int
	preflightRegistrationFailures int
	otpTimeoutStopped             bool
}

func newRuntimeTaskStats() *runtimeTaskStats {
	return &runtimeTaskStats{
		failCategories:               make(map[string]int),
		failureSamples:               make(map[string]int),
		preflightFailureCategories:   make(map[string]int),
		networkStageFailures:         make(map[string]int),
		graphFailureCategories:       make(map[string]int),
		passwordSetFailureCategories: make(map[string]int),
		emailServiceFailureDetails:   make(map[string]int),
		riskFailureDetails:           make(map[string]int),
		sendOTPDiagnosticCounts:      make(map[string]map[string]int),
	}
}

func (s *runtimeTaskStats) RecordFailure(errorMsg string, passwordSet bool, stageHint string) string {
	if s == nil {
		return classifyError(errorMsg)
	}
	reason := classifyError(errorMsg)
	s.failCategories[reason]++
	if passwordSet {
		s.passwordSetFailures++
		s.passwordSetFailureCategories[postRegistrationFailureDetail(errorMsg, reason)]++
	}
	if normalized := normalizeFailureSample(errorMsg); normalized != "" {
		s.failureSamples[normalized]++
	}
	if isSendOTPBlockedError(errorMsg) {
		s.sendOTPBlockedFailures++
	}
	if detail := riskFailureDetail(errorMsg, reason); detail != "" {
		s.riskFailureDetails[detail]++
	}
	if detail := emailServiceFailureDetail(errorMsg, reason); detail != "" {
		s.emailServiceFailureDetails[detail]++
	}
	if diag, ok := parseSendOTPDiagnostics(errorMsg); ok {
		s.recordSendOTPDiagnostics(diag)
	}
	if reason == "网络/代理问题" || isProxyNetworkError(errorMsg) {
		stage := strings.TrimSpace(stageHint)
		if stage == "" {
			stage = classifyNetworkErrorStage(errorMsg)
		}
		s.networkStageFailures[stage]++
	}
	return reason
}

func (s *runtimeTaskStats) RecordPreflightEmailCreateFailure(errorMsg string) {
	if s == nil {
		return
	}
	s.preflightEmailCreateFailures++
	s.preflightFailureCategories["邮箱创建失败"]++
	s.emailServiceFailureDetails["邮箱创建失败"]++
	if normalized := normalizeFailureSample("邮箱创建失败: " + errorMsg); normalized != "" {
		s.failureSamples[normalized]++
	}
}

func (s *runtimeTaskStats) RecordPreflightProxySelectFailure(errorMsg string) {
	if s == nil {
		return
	}
	s.preflightProxySelectFailures++
	s.preflightFailureCategories["代理预选失败"]++
	if normalized := normalizeFailureSample("代理预选失败: " + errorMsg); normalized != "" {
		s.failureSamples[normalized]++
	}
}

func (s *runtimeTaskStats) RecordPreflightRegistrationFailure(errorMsg string) {
	if s == nil {
		return
	}
	s.preflightRegistrationFailures++
	s.preflightFailureCategories["预注册失败"]++
	if detail := riskFailureDetail(errorMsg, classifyError(errorMsg)); detail != "" {
		s.riskFailureDetails[detail]++
	}
	if normalized := normalizeFailureSample("预注册失败: " + errorMsg); normalized != "" {
		s.failureSamples[normalized]++
	}
}

func (s *runtimeTaskStats) RecordRegisteredSkip() {
	if s != nil {
		s.registeredSkipCount++
	}
}

func (s *runtimeTaskStats) RecordGraphFailure(reason string) {
	if s != nil {
		s.graphResolveFailures++
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = "Graph响应异常"
		}
		s.graphFailureCategories[reason]++
		if normalized := normalizeFailureSample(reason); normalized != "" {
			s.failureSamples["Graph: "+normalized]++
		}
	}
}

func (s *runtimeTaskStats) recordSendOTPDiagnostics(diag map[string]string) {
	if s == nil {
		return
	}
	for _, key := range []string{"provider", "domain", "emailProxy", "proxy"} {
		value := strings.TrimSpace(diag[key])
		if value == "" {
			continue
		}
		if s.sendOTPDiagnosticCounts[key] == nil {
			s.sendOTPDiagnosticCounts[key] = make(map[string]int)
		}
		s.sendOTPDiagnosticCounts[key][value]++
	}
}

func (s *runtimeTaskStats) DiagnosticsSnapshot(clashQuarantined, poolQuarantined, topN int) TaskDiagnostics {
	if s == nil {
		return TaskDiagnostics{}
	}
	return TaskDiagnostics{
		OTPFailures: diagnosticGroup(map[string]int{
			"验证码发送失败":              s.failCategories["验证码发送失败"],
			"send-otp TES/BLOCKED": s.sendOTPBlockedFailures,
			"验证码无效":                s.failCategories["验证码无效"],
			"验证码超时":                s.failCategories["验证码超时"],
		}),
		PostRegistrationFailures: diagnosticGroup(s.passwordSetFailureCategories),
		NetworkProxyFailures:     diagnosticGroup(s.networkStageFailures),
		GraphFailures:            diagnosticGroup(s.graphFailureCategories),
		RiskFailures:             diagnosticGroup(s.riskFailureDetailsSnapshot(clashQuarantined, poolQuarantined)),
		EmailServiceFailures:     diagnosticGroup(s.emailServiceFailureDetails),
		ProxyFailures: diagnosticGroup(map[string]int{
			"Clash网络错误": s.clashNetworkErrors,
			"Clash风控错误": s.clashRiskFailures,
			"Clash临时拉黑": clashQuarantined,
			"代理池网络错误":   s.poolNetworkErrors,
			"代理池临时拉黑":   poolQuarantined,
		}),
		SendOTPDiagnostics: s.topSendOTPDiagnostics(topN),
		TopFailures:        sortedDiagnosticTopItems(s.failureSamples, topN),
	}
}

func (s *runtimeTaskStats) riskFailureDetailsSnapshot(clashQuarantined, poolQuarantined int) map[string]int {
	details := make(map[string]int, len(s.riskFailureDetails)+2)
	for name, count := range s.riskFailureDetails {
		details[name] = count
	}
	details["Clash风控节点"] = s.clashRiskFailures
	details["被临时拉黑代理"] = clashQuarantined + poolQuarantined
	return details
}

func (s *runtimeTaskStats) topSendOTPDiagnostics(topN int) map[string][]DiagnosticTopItem {
	out := make(map[string][]DiagnosticTopItem, len(s.sendOTPDiagnosticCounts))
	for _, key := range []string{"provider", "domain", "emailProxy", "proxy"} {
		items := sortedDiagnosticTopItems(s.sendOTPDiagnosticCounts[key], topN)
		if len(items) > 0 {
			out[key] = items
		}
	}
	return out
}

func sortedDiagnosticTopItems(counts map[string]int, n int) []DiagnosticTopItem {
	if n <= 0 {
		n = 10
	}
	if len(counts) == 0 {
		return nil
	}
	items := make([]DiagnosticTopItem, 0, len(counts))
	for label, count := range counts {
		if strings.TrimSpace(label) == "" || count <= 0 {
			continue
		}
		items = append(items, DiagnosticTopItem{Label: label, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Label < items[j].Label
		}
		return items[i].Count > items[j].Count
	})
	if n > len(items) {
		n = len(items)
	}
	return items[:n]
}

func recordEmailProviderOTPStat(provider string, result map[string]interface{}) {
	if result == nil {
		return
	}
	received, _ := result["otpReceived"].(bool)
	if !received {
		return
	}
	if err := storage.RecordEmailProviderOTPReceived(provider); err != nil {
		log.Printf("[Kiro] 记录邮箱渠道收码统计失败: provider=%s err=%v", provider, err)
	}
}

func recordEmailProviderSuccessStat(provider string, result map[string]interface{}) {
	if result == nil || result["status"] != "success" {
		return
	}
	emailAddr, _ := result["email"].(string)
	if err := storage.RecordEmailProviderRegistrationSuccess(provider, emailAddr); err != nil {
		log.Printf("[Kiro] 记录邮箱渠道注册成功统计失败: provider=%s email=%s err=%v", provider, emailAddr, err)
	}
}

func recordEmailProviderDomainAttemptStat(provider, emailAddr string) {
	if strings.TrimSpace(emailAddr) == "" {
		return
	}
	if err := storage.RecordEmailProviderDomainAttempt(provider, emailAddr); err != nil {
		log.Printf("[Kiro] 记录邮箱渠道域名尝试统计失败: provider=%s email=%s err=%v", provider, emailAddr, err)
	}
}

func (s *runtimeTaskStats) ProgressSummary(completed, success, failed, topN int) string {
	if topN <= 0 {
		topN = 3
	}
	rate := 0.0
	if completed > 0 {
		rate = float64(success) / float64(completed) * 100
	}
	return fmt.Sprintf("[Kiro] 进度汇总: 总计=%d, 成功=%d, 失败=%d, 成功率=%.1f%%, 已注册跳过=%d, Graph失败=%d, 网络错误=%d, passwordSet失败=%d, 前置失败=%d, 代理池拉黑=%d, Top失败=%s",
		completed, success, failed, rate, s.registeredSkipCount, s.graphResolveFailures, s.totalNetworkErrors(), s.passwordSetFailures, s.totalPreflightFailures(), proxy.QuarantinedPoolProxyCount(), s.topFailures(topN))
}

func (s *runtimeTaskStats) totalPreflightFailures() int {
	if s == nil {
		return 0
	}
	return s.preflightEmailCreateFailures + s.preflightProxySelectFailures + s.preflightRegistrationFailures
}

func (s *runtimeTaskStats) totalNetworkErrors() int {
	if s == nil {
		return 0
	}
	total := s.clashNetworkErrors + s.poolNetworkErrors
	for _, count := range s.networkStageFailures {
		total += count
	}
	return total
}

func (s *runtimeTaskStats) topFailures(n int) string {
	if s == nil || len(s.failureSamples) == 0 {
		return "-"
	}
	type entry struct {
		text  string
		count int
	}
	entries := make([]entry, 0, len(s.failureSamples))
	for text, count := range s.failureSamples {
		entries = append(entries, entry{text: text, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].text < entries[j].text
		}
		return entries[i].count > entries[j].count
	})
	if n > len(entries) {
		n = len(entries)
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, fmt.Sprintf("%s:%d", entries[i].text, entries[i].count))
	}
	return strings.Join(parts, "；")
}

func (s *runtimeTaskStats) logPreflightFailureSummary() {
	if s == nil || len(s.preflightFailureCategories) == 0 {
		return
	}
	log.Printf("[Kiro] 前置补齐失败:")
	type preflightEntry struct {
		name  string
		count int
	}
	entries := make([]preflightEntry, 0, len(s.preflightFailureCategories))
	for name, count := range s.preflightFailureCategories {
		entries = append(entries, preflightEntry{name, count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].name < entries[j].name
		}
		return entries[i].count > entries[j].count
	})
	for _, entry := range entries {
		log.Printf("[Kiro]   %s: %d", entry.name, entry.count)
	}
}

func normalizeFailureSample(errorMsg string) string {
	s := strings.TrimSpace(errorMsg)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}

type outlookRegistrationAddressTracker struct {
	attempted  map[string]struct{}
	registered map[string]struct{}
}

func newOutlookRegistrationAddressTracker() *outlookRegistrationAddressTracker {
	return &outlookRegistrationAddressTracker{
		attempted:  make(map[string]struct{}),
		registered: make(map[string]struct{}),
	}
}

func (t *outlookRegistrationAddressTracker) key(acc email.OutlookAccount) string {
	if t == nil {
		return ""
	}
	addr := strings.TrimSpace(acc.RegistrationEmail)
	if addr == "" {
		addr = strings.TrimSpace(acc.Email)
	}
	return strings.ToLower(addr)
}

func (t *outlookRegistrationAddressTracker) ShouldSkip(acc email.OutlookAccount) bool {
	key := t.key(acc)
	if key == "" {
		return false
	}
	if _, ok := t.attempted[key]; ok {
		return true
	}
	if _, ok := t.registered[key]; ok {
		return true
	}
	return false
}

func (t *outlookRegistrationAddressTracker) MarkAttempt(acc email.OutlookAccount) {
	if key := t.key(acc); key != "" {
		t.attempted[key] = struct{}{}
	}
}

func (t *outlookRegistrationAddressTracker) MarkRegistered(acc email.OutlookAccount) {
	if key := t.key(acc); key != "" {
		t.registered[key] = struct{}{}
	}
}

type outlookOTPTimeoutStreak struct {
	count int
	limit int
}

func (s *outlookOTPTimeoutStreak) Record(failReason string) bool {
	if s.limit <= 0 {
		s.limit = 5
	}
	if strings.TrimSpace(failReason) != "验证码超时" {
		s.count = 0
		return false
	}
	s.count++
	return s.count >= s.limit
}

func concurrentStartStagger(idx int, concurrency int) time.Duration {
	if concurrency <= 1 {
		return 0
	}
	base := time.Duration(idx%concurrency) * concurrentStartStaggerStep
	jitterMs := rand.Intn(int(concurrentStartStaggerJitterMax/time.Millisecond) + 1)
	return base + time.Duration(jitterMs)*time.Millisecond
}

type registrarStartPacer struct {
	mu        sync.Mutex
	step      time.Duration
	nextStart time.Time
}

func newRegistrarStartPacer(step time.Duration) *registrarStartPacer {
	return &registrarStartPacer{step: step}
}

func (p *registrarStartPacer) ReserveDelay(now time.Time) time.Duration {
	if p == nil || p.step <= 0 {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.nextStart.IsZero() || !now.Before(p.nextStart) {
		p.nextStart = now.Add(p.step)
		return 0
	}
	delay := p.nextStart.Sub(now)
	p.nextStart = p.nextStart.Add(p.step)
	return delay
}

func shouldEnableRegistrarStartPacing(concurrency int, proxyMode string, rawProxy string) bool {
	return false
}

type runtimeProxyEgressStartPacer struct {
	mu        sync.Mutex
	step      time.Duration
	nextStart map[string]time.Time
}

func newRuntimeProxyEgressStartPacer(step time.Duration) *runtimeProxyEgressStartPacer {
	return &runtimeProxyEgressStartPacer{
		step:      step,
		nextStart: make(map[string]time.Time),
	}
}

func (p *runtimeProxyEgressStartPacer) ReserveDelay(egressIP string, now time.Time) time.Duration {
	if p == nil || p.step <= 0 {
		return 0
	}
	egressIP = strings.TrimSpace(egressIP)
	if egressIP == "" {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.nextStart == nil {
		p.nextStart = make(map[string]time.Time)
	}
	next := p.nextStart[egressIP]
	if next.IsZero() || !now.Before(next) {
		p.nextStart[egressIP] = now.Add(p.step)
		return 0
	}
	delay := next.Sub(now)
	p.nextStart[egressIP] = next.Add(p.step)
	return delay
}

type taskIdleConnectionCloser interface {
	CloseIdleConnections()
}

func closeTaskConfigIdleConnections(cfg *core.Config) {
	if cfg == nil {
		return
	}
	if closer, ok := cfg.TempEmailService.(taskIdleConnectionCloser); ok {
		closer.CloseIdleConnections()
	}
	if closer, ok := any(cfg.MoeMailProvider).(taskIdleConnectionCloser); ok {
		closer.CloseIdleConnections()
	}
	if closer, ok := any(cfg.CloudMailProvider).(taskIdleConnectionCloser); ok {
		closer.CloseIdleConnections()
	}
}

// StartTaskRequest 启动任务请求
type StartTaskRequest struct {
	Count                    int                              `json:"count"`
	SuccessTarget            int                              `json:"successTarget"`
	Concurrency              int                              `json:"concurrency"`
	Delay                    int                              `json:"delay"`
	DomainExplorationPercent int                              `json:"domainExplorationPercent"`
	ProxyExplorationPercent  int                              `json:"proxyExplorationPercent"`
	RetryCount               int                              `json:"retryCount"`
	OTPTimeout               int                              `json:"otpTimeout"`
	ReuseFailedEmail         bool                             `json:"reuseFailedEmail"`
	OutputPath               string                           `json:"outputPath"`
	EmailProviders           []string                         `json:"emailProviders"`    // 多邮箱渠道，按注册 attempt 轮询
	MoeMailDomains           []string                         `json:"moemailDomains"`    // 选中的域名列表
	MoeMailConfigs           map[string][]email.MoeMailConfig `json:"moemailConfigs"`    // 域名 -> 配置列表映射
	MoeMailRandomMode        bool                             `json:"moemailRandomMode"` // 是否为随机模式

	CloudMailDomains    []string                           `json:"cloudmailDomains"`
	CloudMailConfigs    map[string][]email.CloudMailConfig `json:"cloudmailConfigs"`
	CloudMailRandomMode bool                               `json:"cloudmailRandomMode"`
}

type emailProviderSelector struct {
	mu                  sync.Mutex
	providers           []string
	next                int
	createFailureStreak map[string]int
	cooldownUntil       map[string]time.Time
}

const (
	emailProviderHistoricalMinOTP               = 5
	emailProviderHistoricalMinSuccess           = 3
	emailProviderHistoricalMinSuccessRate       = 0.50
	emailProviderCreateFailureCooldownThreshold = 8
	emailProviderCreateFailureCooldown          = 90 * time.Second
)

func newEmailProviderSelector(providers []string) *emailProviderSelector {
	return &emailProviderSelector{
		providers:           append([]string(nil), providers...),
		createFailureStreak: make(map[string]int),
		cooldownUntil:       make(map[string]time.Time),
	}
}

func (s *emailProviderSelector) Next() string {
	provider, _ := s.NextWithContext(context.Background())
	return provider
}

func (s *emailProviderSelector) NextWithContext(ctx context.Context) (string, error) {
	if s == nil {
		return "outlook", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		provider, wait, ok := s.nextAvailable(time.Now())
		if ok {
			return provider, nil
		}
		if wait <= 0 {
			wait = 100 * time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *emailProviderSelector) nextAvailable(now time.Time) (string, time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.providers) == 0 {
		return "outlook", 0, true
	}
	var minWait time.Duration
	for offset := 0; offset < len(s.providers); offset++ {
		idx := (s.next + offset) % len(s.providers)
		provider := s.providers[idx]
		until := s.cooldownUntil[provider]
		if until.After(now) {
			wait := until.Sub(now)
			if minWait == 0 || wait < minWait {
				minWait = wait
			}
			continue
		}
		if !until.IsZero() {
			delete(s.cooldownUntil, provider)
		}
		s.next = idx + 1
		return provider, 0, true
	}
	return "", minWait, false
}

func (s *emailProviderSelector) ReportCreateFailure(provider string) bool {
	if s == nil {
		return false
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || provider == "outlook" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createFailureStreak == nil {
		s.createFailureStreak = make(map[string]int)
	}
	if s.cooldownUntil == nil {
		s.cooldownUntil = make(map[string]time.Time)
	}
	s.createFailureStreak[provider]++
	if s.createFailureStreak[provider] < emailProviderCreateFailureCooldownThreshold {
		return false
	}
	s.createFailureStreak[provider] = 0
	s.cooldownUntil[provider] = time.Now().Add(emailProviderCreateFailureCooldown)
	return true
}

func (s *emailProviderSelector) ReportCreateSuccess(provider string) {
	if s == nil {
		return
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.createFailureStreak, provider)
	delete(s.cooldownUntil, provider)
}

func normalizeStartEmailProviders(providers []string) ([]string, error) {
	return storage.NormalizeRegistrationEmailProviders(providers)
}

func effectiveEmailProvidersForBatch(providers []string) []string {
	return effectiveEmailProvidersForBatchByStats(providers, storage.GetEmailProviderStats())
}

func effectiveEmailProvidersForBatchByStats(providers []string, stats []storage.EmailProviderStat) []string {
	original := append([]string(nil), providers...)
	if len(original) <= 1 {
		return original
	}

	statByProvider := make(map[string]storage.EmailProviderStat, len(stats))
	for _, stat := range stats {
		provider := strings.ToLower(strings.TrimSpace(stat.Provider))
		if provider == "" {
			continue
		}
		statByProvider[provider] = stat
	}

	winners := make([]string, 0, len(original))
	for _, provider := range original {
		key := strings.ToLower(strings.TrimSpace(provider))
		stat, ok := statByProvider[key]
		if !ok {
			continue
		}
		if stat.OTPReceivedCount < emailProviderHistoricalMinOTP || stat.RegistrationSuccessCount < emailProviderHistoricalMinSuccess {
			continue
		}
		successRate := float64(stat.RegistrationSuccessCount) / float64(stat.OTPReceivedCount)
		if successRate >= emailProviderHistoricalMinSuccessRate {
			winners = append(winners, provider)
		}
	}
	if len(winners) == 0 {
		return original
	}
	return winners
}

func startRequestUsesProvider(req StartTaskRequest, provider string) bool {
	providers, err := normalizeStartEmailProviders(req.EmailProviders)
	if err != nil {
		return false
	}
	return emailProviderListContains(providers, provider)
}

func emailProviderListContains(providers []string, provider string) bool {
	for _, item := range providers {
		if item == provider {
			return true
		}
	}
	return false
}

type reusableEmailCandidate struct {
	provider          string
	address           string
	moeMailProvider   *email.MoeMailProvider
	cloudMailProvider *email.CloudMailProvider
	tempEmailService  email.TempEmailService
}

type reusableEmailPool struct {
	mu    sync.Mutex
	items []reusableEmailCandidate
}

// put 回收一个仍可继续尝试注册的临时邮箱。
func (p *reusableEmailPool) put(candidate reusableEmailCandidate) bool {
	if strings.TrimSpace(candidate.provider) == "" || strings.TrimSpace(candidate.address) == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = append(p.items, candidate)
	return true
}

// take 按邮箱提供商领取一个可复用临时邮箱。
func (p *reusableEmailPool) take(provider string) (reusableEmailCandidate, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, candidate := range p.items {
		if candidate.provider != provider {
			continue
		}
		p.items = append(p.items[:i], p.items[i+1:]...)
		return candidate, true
	}
	return reusableEmailCandidate{}, false
}

// applyReusableEmailCandidate 将候选邮箱绑定到本次注册配置。
func applyReusableEmailCandidate(provider string, cfg *core.Config, candidate reusableEmailCandidate) (string, bool) {
	if cfg == nil || candidate.provider != provider {
		return "", false
	}
	switch provider {
	case "moemail":
		if candidate.moeMailProvider == nil {
			return "", false
		}
		cfg.MoeMailProvider = candidate.moeMailProvider
		address := strings.TrimSpace(candidate.moeMailProvider.GetAddress())
		return address, address != ""
	case "cloudmail":
		if candidate.cloudMailProvider == nil {
			return "", false
		}
		cfg.CloudMailProvider = candidate.cloudMailProvider
		address := strings.TrimSpace(candidate.cloudMailProvider.GetAddress())
		return address, address != ""
	default:
		if !isTemporaryEmailProvider(provider) {
			return "", false
		}
		if candidate.tempEmailService == nil {
			return "", false
		}
		cfg.TempEmailService = candidate.tempEmailService
		address := strings.TrimSpace(candidate.tempEmailService.GetAddress())
		return address, address != ""
	}
}

// reusableEmailCandidateFromConfig 从本次注册配置提取可回收的临时邮箱。
func reusableEmailCandidateFromConfig(provider string, cfg *core.Config) (reusableEmailCandidate, bool) {
	if cfg == nil {
		return reusableEmailCandidate{}, false
	}
	switch provider {
	case "moemail":
		if cfg.MoeMailProvider == nil {
			return reusableEmailCandidate{}, false
		}
		address := strings.TrimSpace(cfg.MoeMailProvider.GetAddress())
		return reusableEmailCandidate{provider: provider, address: address, moeMailProvider: cfg.MoeMailProvider}, address != ""
	case "cloudmail":
		if cfg.CloudMailProvider == nil {
			return reusableEmailCandidate{}, false
		}
		address := strings.TrimSpace(cfg.CloudMailProvider.GetAddress())
		return reusableEmailCandidate{provider: provider, address: address, cloudMailProvider: cfg.CloudMailProvider}, address != ""
	default:
		if !isTemporaryEmailProvider(provider) {
			return reusableEmailCandidate{}, false
		}
		if cfg.TempEmailService == nil {
			return reusableEmailCandidate{}, false
		}
		address := strings.TrimSpace(cfg.TempEmailService.GetAddress())
		return reusableEmailCandidate{provider: provider, address: address, tempEmailService: cfg.TempEmailService}, address != ""
	}
}

// effectiveSuccessTarget 返回本次任务的成功目标，未配置时沿用注册数量。
func effectiveSuccessTarget(req StartTaskRequest) int {
	if req.SuccessTarget > 0 {
		return req.SuccessTarget
	}
	return req.Count
}

func shouldUseReusableFailedEmail(req StartTaskRequest) bool {
	return req.ReuseFailedEmail
}

func takeReusableFailedEmail(req StartTaskRequest, pool *reusableEmailPool, provider string) (reusableEmailCandidate, bool) {
	if !shouldUseReusableFailedEmail(req) || provider == "outlook" || pool == nil {
		return reusableEmailCandidate{}, false
	}
	return pool.take(provider)
}

func recycleReusableFailedEmail(req StartTaskRequest, pool *reusableEmailPool, provider string, cfg *core.Config, result map[string]interface{}, killSwitchBlocked bool) (reusableEmailCandidate, bool) {
	if !shouldUseReusableFailedEmail(req) || pool == nil || killSwitchBlocked || result == nil || result["status"] == "success" {
		return reusableEmailCandidate{}, false
	}
	errorMsg, _ := result["error"].(string)
	if isTemporaryEmailProvider(provider) && isSendOTPMailboxRejectedError(errorMsg) {
		return reusableEmailCandidate{}, false
	}
	if !shouldRecycleReusableEmail(result) {
		return reusableEmailCandidate{}, false
	}
	candidate, ok := reusableEmailCandidateFromConfig(provider, cfg)
	if !ok {
		return reusableEmailCandidate{}, false
	}
	if !pool.put(candidate) {
		return reusableEmailCandidate{}, false
	}
	return candidate, true
}

// StartTask 公开方法（包装器）
func StartTask(req StartTaskRequest) map[string]interface{} {
	req.DomainExplorationPercent = clampDomainExplorationPercent(req.DomainExplorationPercent)
	req.ProxyExplorationPercent = clampDomainExplorationPercent(req.ProxyExplorationPercent)
	return startTask(req)
}

func buildAvailableOutlookAccounts(storedAccounts []map[string]interface{}) []email.OutlookAccount {
	clean := make([]email.OutlookAccount, 0, len(storedAccounts))
	deferred := make([]email.OutlookAccount, 0)
	seen := make(map[string]struct{})
	for _, acc := range storedAccounts {
		registered, _ := acc["registered"].(bool)
		if registered {
			continue
		}
		emailAddr, _ := acc["email"].(string)
		password, _ := acc["password"].(string)
		clientID, _ := acc["clientId"].(string)
		refreshToken, _ := acc["refreshToken"].(string)
		registrationEmail, _ := acc["registrationEmail"].(string)
		graphPrimaryEmail, _ := acc["graphPrimaryEmail"].(string)
		graphAliasVerified, _ := acc["graphAliasVerified"].(bool)
		graphResolvedAt, _ := acc["graphResolvedAt"].(string)
		outlookAccount := email.OutlookAccount{
			Email:              emailAddr,
			Password:           password,
			ClientID:           clientID,
			RefreshToken:       refreshToken,
			RegistrationEmail:  registrationEmail,
			GraphPrimaryEmail:  graphPrimaryEmail,
			GraphAliasVerified: graphAliasVerified,
			GraphResolvedAt:    graphResolvedAt,
		}
		dedupeKey := strings.ToLower(strings.TrimSpace(registrationEmail))
		if dedupeKey == "" {
			dedupeKey = strings.ToLower(strings.TrimSpace(emailAddr))
		}
		if dedupeKey != "" {
			if _, ok := seen[dedupeKey]; ok {
				continue
			}
			seen[dedupeKey] = struct{}{}
		}
		failReason, _ := acc["failReason"].(string)
		if shouldSkipOutlookAccountForPreviousFailure(failReason) {
			continue
		}
		if shouldDeferOutlookAccountForPreviousFailure(failReason) {
			deferred = append(deferred, outlookAccount)
			continue
		}
		clean = append(clean, outlookAccount)
	}
	return append(clean, deferred...)
}

func shouldDeferOutlookAccountForPreviousFailure(failReason string) bool {
	switch strings.TrimSpace(failReason) {
	case "IP/指纹风控", "验证码发送失败", "验证码超时", "Graph网络错误", "Graph响应异常":
		return true
	default:
		return false
	}
}

func shouldSkipOutlookAccountForPreviousFailure(failReason string) bool {
	switch strings.TrimSpace(failReason) {
	case "Graph Token失效", "Graph权限错误", "账号封禁", "邮箱已注册", "异常邮箱":
		return true
	default:
		return false
	}
}

func shouldConsumeOutlookAccountAfterGraphResolutionFailure(failReason string) bool {
	switch strings.TrimSpace(failReason) {
	case "Graph Token失效", "Graph权限错误":
		return true
	default:
		return false
	}
}

func newCachedOutlookGraphProfileResolver(resolver outlookGraphProfileResolver) outlookGraphProfileResolver {
	type cachedProfile struct {
		profile email.OutlookGraphProfile
		err     error
	}
	cache := make(map[string]cachedProfile)
	var mu sync.Mutex
	return func(acc email.OutlookAccount, proxyURL string) (email.OutlookGraphProfile, error) {
		if resolver == nil {
			return email.OutlookGraphProfile{}, fmt.Errorf("Outlook Graph profile resolver is nil")
		}
		key := strings.ToLower(strings.TrimSpace(acc.ClientID)) + "\x00" + strings.TrimSpace(acc.RefreshToken)
		if strings.TrimSpace(key) == "\x00" {
			return resolver(acc, proxyURL)
		}
		mu.Lock()
		if item, ok := cache[key]; ok {
			mu.Unlock()
			return item.profile, item.err
		}
		mu.Unlock()
		profile, err := resolver(acc, proxyURL)
		if err == nil {
			mu.Lock()
			cache[key] = cachedProfile{profile: profile}
			mu.Unlock()
		}
		return profile, err
	}
}

type outlookGraphProfileResolver func(email.OutlookAccount, string) (email.OutlookGraphProfile, error)

func resolveOutlookGraphRegistrationEmailForTask(acc email.OutlookAccount, emailProxy, mode string, resolver outlookGraphProfileResolver) (email.OutlookAccount, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = storage.OutlookGraphRegistrationEmailAuto
	}
	imported := strings.TrimSpace(acc.Email)
	if mode == storage.OutlookGraphRegistrationEmailImported {
		acc.RegistrationEmail = imported
		return acc, nil
	}
	if mode == storage.OutlookGraphRegistrationEmailAuto && strings.TrimSpace(acc.RegistrationEmail) != "" {
		return acc, nil
	}
	if resolver == nil {
		acc.RegistrationEmail = imported
		return acc, fmt.Errorf("Outlook Graph profile resolver is nil")
	}
	profile, err := resolver(acc, emailProxy)
	if err != nil {
		acc.RegistrationEmail = imported
		return acc, err
	}
	primary := strings.TrimSpace(profile.PrimaryEmail)
	acc.GraphPrimaryEmail = primary
	acc.GraphAliasVerified = profile.HasAliasData() && profile.HasAddress(imported)
	acc.GraphResolvedAt = time.Now().Format("2006-01-02 15:04:05")
	switch mode {
	case storage.OutlookGraphRegistrationEmailPrimary:
		if primary != "" {
			acc.RegistrationEmail = primary
		} else {
			acc.RegistrationEmail = imported
		}
	case storage.OutlookGraphRegistrationEmailAuto:
		if profile.HasAliasData() && !profile.HasAddress(imported) && primary != "" {
			acc.RegistrationEmail = primary
		} else {
			acc.RegistrationEmail = imported
		}
	default:
		acc.RegistrationEmail = imported
	}
	logOutlookGraphRegistrationChoice(imported, acc.GraphPrimaryEmail, acc.RegistrationEmail, mode)
	return acc, nil
}

func logOutlookGraphRegistrationChoice(imported, primary, registration, mode string) {
	if strings.TrimSpace(primary) == "" {
		log.Printf("[Kiro] Outlook Graph 注册邮箱: %s（策略: %s，未获取到主邮箱）", registration, mode)
		return
	}
	if strings.EqualFold(registration, imported) && !strings.EqualFold(imported, primary) {
		log.Printf("[Kiro] Outlook Graph 主邮箱: %s，注册邮箱: %s（使用导入别名，策略: %s）", primary, registration, mode)
		return
	}
	if !strings.EqualFold(registration, imported) {
		log.Printf("[Kiro] Outlook Graph 主邮箱: %s，注册邮箱: %s（导入邮箱: %s，策略: %s）", primary, registration, imported, mode)
		return
	}
	log.Printf("[Kiro] Outlook Graph 主邮箱: %s，注册邮箱: %s（策略: %s）", primary, registration, mode)
}

func prepareMoeMailStartRequest(req StartTaskRequest, loadSavedConfigs func() []email.MoeMailConfig) StartTaskRequest {
	if !startRequestUsesProvider(req, "moemail") || len(req.MoeMailDomains) == 0 || loadSavedConfigs == nil {
		return req
	}

	missingConfig := false
	for _, domain := range req.MoeMailDomains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		if len(req.MoeMailConfigs[domain]) == 0 {
			missingConfig = true
			break
		}
	}
	if !missingConfig {
		return req
	}

	savedConfigs := loadSavedConfigs()
	if len(savedConfigs) == 0 {
		return req
	}

	merged := make(map[string][]email.MoeMailConfig, len(req.MoeMailConfigs)+len(req.MoeMailDomains))
	for domain, configs := range req.MoeMailConfigs {
		merged[domain] = append([]email.MoeMailConfig(nil), configs...)
	}
	for _, domain := range req.MoeMailDomains {
		domain = strings.TrimSpace(domain)
		if domain == "" || len(merged[domain]) > 0 {
			continue
		}
		merged[domain] = append([]email.MoeMailConfig(nil), savedConfigs...)
	}
	req.MoeMailConfigs = merged
	return req
}

func validateMoeMailDeliverability(req StartTaskRequest, hasMX func(string) (bool, error)) error {
	if !startRequestUsesProvider(req, "moemail") || hasMX == nil {
		return nil
	}
	for _, domain := range req.MoeMailDomains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		ok, err := hasMX(domain)
		if err != nil || !ok {
			if err != nil {
				return fmt.Errorf("MoeMail 域名 %s 缺少 MX 或 DNS 查询失败: %w；send-otp 即使成功也无法投递验证码", domain, err)
			}
			return fmt.Errorf("MoeMail 域名 %s 缺少 MX；send-otp 即使成功也无法投递验证码", domain)
		}
	}
	return nil
}

func domainHasMX(domain string) (bool, error) {
	records, err := net.LookupMX(domain)
	if err != nil {
		return false, err
	}
	return len(records) > 0, nil
}

// startTask 启动注册任务（私有方法）
func startTask(req StartTaskRequest) map[string]interface{} {
	Manager.mu.Lock()
	if Manager.running {
		Manager.mu.Unlock()
		return map[string]interface{}{"error": "任务正在运行中"}
	}

	req = prepareMoeMailStartRequest(req, email.GetMoeMailConfigs)

	emailProviders, err := normalizeStartEmailProviders(req.EmailProviders)
	if err != nil {
		Manager.mu.Unlock()
		return map[string]interface{}{"error": err.Error()}
	}
	req.EmailProviders = emailProviders
	effectiveTarget := effectiveSuccessTarget(req)

	var outlookAccounts []email.OutlookAccount

	if startRequestUsesProvider(req, "moemail") {
		// MoeMail 模式：验证域名和配置
		if len(req.MoeMailDomains) == 0 {
			Manager.mu.Unlock()
			return map[string]interface{}{"error": "请选择至少一个域名"}
		}
		if len(req.MoeMailConfigs) == 0 {
			Manager.mu.Unlock()
			return map[string]interface{}{"error": "MoeMail 配置缺失"}
		}
		if err := validateMoeMailDeliverability(req, domainHasMX); err != nil {
			Manager.mu.Unlock()
			return map[string]interface{}{"error": err.Error()}
		}
		// MoeMail 不需要预先加载账号，每次任务动态生成
	}
	if startRequestUsesProvider(req, "cloudmail") {
		if len(req.CloudMailDomains) == 0 {
			Manager.mu.Unlock()
			return map[string]interface{}{"error": "请选择至少一个 cloud-mail 域名"}
		}
		if len(req.CloudMailConfigs) == 0 {
			Manager.mu.Unlock()
			return map[string]interface{}{"error": "cloud-mail 配置缺失"}
		}
	}
	if startRequestUsesProvider(req, "outlook") {
		// Outlook 模式：加载账号列表
		storedAccounts := storage.GetAccountsCached()
		onlyOutlook := len(emailProviders) == 1
		if len(storedAccounts) == 0 && onlyOutlook {
			Manager.mu.Unlock()
			return map[string]interface{}{"error": "请先添加微软邮箱账号"}
		}

		// 筛选未注册的账号；上次已在 send-otp/TES 阶段失败的账号延后使用，
		// 避免每次启动都卡在同一个已知被目标拒绝的邮箱/域组合。
		outlookAccounts = buildAvailableOutlookAccounts(storedAccounts)

		if len(outlookAccounts) == 0 && onlyOutlook {
			Manager.mu.Unlock()
			return map[string]interface{}{"error": "没有可用的 Outlook 账号（所有账号已注册成功）"}
		}

		if onlyOutlook && len(outlookAccounts) < effectiveTarget {
			Manager.mu.Unlock()
			// 返回确认类型响应，由前端弹窗让用户选择是否继续
			return map[string]interface{}{
				"confirm":   "outlook_insufficient",
				"message":   fmt.Sprintf("可用 Outlook 账号不足: 需要 %d, 仅有 %d。是否使用 %d 个可用账号继续？", effectiveTarget, len(outlookAccounts), len(outlookAccounts)),
				"available": len(outlookAccounts),
			}
		}
	}

	// 初始化状态
	Manager.running = true
	Manager.stopCh = make(chan struct{})
	Manager.total = effectiveTarget
	Manager.completed = 0
	Manager.success = 0
	Manager.failed = 0
	Manager.successTarget = req.SuccessTarget
	Manager.successTargetEnabled = req.SuccessTarget > 0
	Manager.results = nil
	Manager.diagnostics = TaskDiagnostics{}
	Manager.startTime = time.Now()
	Manager.mu.Unlock()

	// 清空日志
	Manager.logsMu.Lock()
	Manager.logs = nil
	Manager.logsMu.Unlock()

	// 后台执行
	go runBatch(req, outlookAccounts)

	return map[string]interface{}{"status": "started"}
}

// StopTask 停止任务（强制取消所有 HTTP 请求）
func StopTask(force bool) map[string]interface{} {
	Manager.mu.Lock()
	if !Manager.running {
		Manager.mu.Unlock()
		return map[string]interface{}{"error": "没有正在运行的任务"}
	}

	select {
	case <-Manager.stopCh:
	default:
		close(Manager.stopCh)
	}

	// 强制取消所有进行中的 HTTP 请求
	if Manager.cancelFunc != nil {
		Manager.cancelFunc()
	}

	Manager.running = false
	log.Println("[Kiro] 任务已强制停止，所有请求已取消")
	Manager.mu.Unlock()
	return map[string]interface{}{"status": "force_stopped"}
}

// runBatch 执行批量注册
func runBatch(req StartTaskRequest, outlookAccounts []email.OutlookAccount) {
	// 创建可取消的 context，停止时立即中断所有 HTTP 请求
	taskCtx, taskCancel := context.WithCancel(context.Background())
	defer taskCancel()

	Manager.mu.Lock()
	Manager.cancelFunc = taskCancel
	Manager.mu.Unlock()

	successTarget := effectiveSuccessTarget(req)
	successTargetMode := req.SuccessTarget > 0
	displayTotal := req.Count
	if successTargetMode {
		displayTotal = successTarget
		log.Printf("[Kiro] 成功目标模式: 目标成功 %d 个，忽略注册数量 %d", successTarget, req.Count)
	}

	defer func() {
		Manager.mu.Lock()
		Manager.running = false
		Manager.cancelFunc = nil
		Manager.mu.Unlock()

		// 关闭日志文件
		if err := Manager.CloseLogFile(); err != nil {
			log.Printf("[Kiro] 日志文件关闭失败: %v", err)
		}
	}()

	emailProviders, err := normalizeStartEmailProviders(req.EmailProviders)
	if err != nil {
		log.Printf("[Kiro] 邮箱渠道配置无效: %v", err)
		Manager.mu.Lock()
		Manager.completed = successTarget
		Manager.failed = successTarget
		Manager.mu.Unlock()
		return
	}
	req.EmailProviders = emailProviders
	effectiveEmailProviders := effectiveEmailProvidersForBatch(emailProviders)
	providerSelector := newEmailProviderSelector(effectiveEmailProviders)
	onlyOutlookProvider := len(effectiveEmailProviders) == 1 && effectiveEmailProviders[0] == "outlook"
	log.Printf("[Kiro] 邮箱渠道轮询: %s", strings.Join(emailProviders, ","))
	if strings.Join(effectiveEmailProviders, ",") != strings.Join(emailProviders, ",") {
		log.Printf("[Kiro] 邮箱渠道历史优选: %s", strings.Join(effectiveEmailProviders, ","))
	}

	outDir := req.OutputPath
	if outDir == "" {
		outDir = storage.GetResultOutputDir()
	}
	os.MkdirAll(outDir, 0755)

	// 初始化日志文件持久化（失败不阻断任务，降级为仅内存日志）
	if err := Manager.InitLogFile(outDir); err != nil {
		log.Printf("[Kiro] 日志文件初始化失败，降级为仅内存日志: %v", err)
	}

	taskConfig := core.NewConfig()
	taskConfig.EmailProxy = storage.GetEmailProxy()
	taskConfig.OutlookScope = storage.GetOutlookScope()
	taskConfig.OTPTimeout = req.OTPTimeout
	if taskConfig.OTPTimeout < 30 {
		taskConfig.OTPTimeout = 60
	}
	if taskConfig.EmailProxy == "" {
		log.Printf("[Kiro] 邮箱代理: 直连")
	} else {
		log.Printf("[Kiro] 邮箱代理: 已启用")
	}

	proxyMode := storage.GetProxyMode()
	failConfig := func(message string) {
		log.Printf("[Kiro] %s，任务终止", message)
		Manager.mu.Lock()
		Manager.completed = successTarget
		Manager.failed = successTarget
		Manager.mu.Unlock()
	}

	var clashConfig proxy.ClashConfig
	clashEnabled := false
	normalClashAssist := false
	switch proxyMode {
	case storage.ProxyModeNone:
		log.Printf("[Kiro] 代理模式: 直连")
	case storage.ProxyModeNormal:
		taskConfig.Proxy = storage.GetProxy()
		if taskConfig.Proxy == "" {
			failConfig("普通代理模式已启用但代理为空")
			return
		}
		if taskConfig.Proxy != "" && proxy.HasURLTemplate(taskConfig.Proxy) {
			log.Printf("[Kiro] 代理模式: 普通代理模板，注册时将动态生成会话代理")
		} else if taskConfig.Proxy != "" {
			log.Printf("[Kiro] 代理模式: 普通代理")
		}
		normalClashConfig := storage.GetClashConfig()
		if shouldEnableNormalClashAssist(proxyMode, taskConfig.Proxy, storage.GetClashProxy(), normalClashConfig, req.Concurrency) {
			clashConfig = normalClashConfig
			normalClashAssist = true
		}
	case storage.ProxyModePool:
		if !proxy.HasEnabled() {
			failConfig("多代理池模式已启用但代理池无启用项")
			return
		}
		log.Printf("[Kiro] 代理模式: 多代理池，将按权重为每个注册任务选择代理")
	case storage.ProxyModeClash:
		taskConfig.Proxy = storage.GetClashProxy()
		if taskConfig.Proxy == "" {
			failConfig("Clash 代理模式已启用但本地代理地址为空")
			return
		}
		clashConfig = storage.GetClashConfig()
		clashConfig.Enabled = true
		clashEnabled = true
	default:
		failConfig("未知代理模式: " + proxyMode)
		return
	}

	var clashClient *proxy.ClashClient
	var clashMu sync.Mutex
	if clashEnabled || normalClashAssist {
		clashClient = proxy.NewClashClient(clashConfig)
		if clashEnabled {
			log.Printf("[Kiro] 已启用 Clash API 自动切换: %s", clashConfig.APIURL)
			if emailProxyUsesClash(taskConfig.EmailProxy, taskConfig.Proxy) {
				log.Printf("[Kiro] 邮箱代理复用 Clash 本地代理；临时邮箱服务出口会受 Clash 当前节点影响")
			}
			if req.Concurrency > 1 {
				log.Printf("[Kiro] Clash 节点为全局状态，注册流程将串行使用代理，避免中途切换节点")
			}
		} else {
			log.Printf("[Kiro] 普通代理启用 Clash 辅助真实性: %s", clashConfig.APIURL)
		}
	}
	killSwitchEnabled := storage.GetKillSwitchEnabled()
	if !killSwitchEnabled {
		log.Println("[Kiro] 熔断级错误自动停止已关闭")
	}

	// 预先准备 MoeMail 域名池
	var moemailDomainPool []string
	var moemailDomainConfigs map[string][]email.MoeMailConfig
	if emailProviderListContains(emailProviders, "moemail") {
		moemailDomainPool = req.MoeMailDomains
		moemailDomainConfigs = req.MoeMailConfigs

		if len(moemailDomainPool) == 0 || len(moemailDomainConfigs) == 0 {
			log.Println("[Kiro] MoeMail 域名或配置为空，任务终止")
			Manager.mu.Lock()
			Manager.running = false
			Manager.mu.Unlock()
			return
		}

		log.Printf("[Kiro] MoeMail 域名池: %v (共 %d 个域名)", moemailDomainPool, len(moemailDomainPool))
	}
	if emailProviderListContains(emailProviders, "outlook") {
		log.Printf("[Kiro] Outlook 读取方式: %s", taskConfig.OutlookScope)
	}
	for _, provider := range emailProviders {
		if isTemporaryEmailProvider(provider) {
			log.Printf("[Kiro] %s 零配置邮箱模式", emailProviderDisplayName(provider))
		}
	}

	// 预先准备 CloudMail 域名池
	var cloudmailDomainPool []string
	var cloudmailDomainConfigs map[string][]email.CloudMailConfig
	if emailProviderListContains(emailProviders, "cloudmail") {
		cloudmailDomainPool = req.CloudMailDomains
		cloudmailDomainConfigs = req.CloudMailConfigs

		if len(cloudmailDomainPool) == 0 || len(cloudmailDomainConfigs) == 0 {
			log.Println("[Kiro] cloud-mail 域名或配置为空，任务终止")
			Manager.mu.Lock()
			Manager.running = false
			Manager.mu.Unlock()
			return
		}

		log.Printf("[Kiro] cloud-mail 域名池: %v (共 %d 个域名)", cloudmailDomainPool, len(cloudmailDomainPool))
	}

	// 统计计数器
	var statsMu sync.Mutex
	var taskDurations []float64
	runtimeStats := newRuntimeTaskStats()
	sendOTPDiagnostics := make([]map[string]string, 0)
	otpTimeoutStopTriggered := false
	publishDiagnostics := func() {
		clashQuarantined := 0
		if clashClient != nil {
			clashQuarantined = clashClient.QuarantinedNodeCount()
		}
		statsMu.Lock()
		runtimeStats.otpTimeoutStopped = otpTimeoutStopTriggered
		diagnostics := runtimeStats.DiagnosticsSnapshot(clashQuarantined, proxy.QuarantinedPoolProxyCount(), 10)
		statsMu.Unlock()
		Manager.SetDiagnostics(diagnostics)
	}
	var otpTimeoutStreak outlookOTPTimeoutStreak
	taskStartTime := time.Now()
	registrarStartPacingEnabled := shouldEnableRegistrarStartPacing(req.Concurrency, proxyMode, taskConfig.Proxy)
	if registrarStartPacingEnabled {
		log.Printf("[Kiro] 真实注册启动节流: 步进 %s，全局错峰进入注册流程", registrarStartPacingStep)
	}
	registrarStartPacer := newRegistrarStartPacer(registrarStartPacingStep)
	waitForRegistrarStartPacing := func(registrationIndex int, displayTotal int) bool {
		if !registrarStartPacingEnabled {
			return true
		}
		delay := registrarStartPacer.ReserveDelay(time.Now())
		if delay <= 0 {
			return true
		}
		log.Printf("[Kiro][%d/%d] 真实注册启动节流等待 %s，降低同批风控聚集", registrationIndex+1, displayTotal, delay.Truncate(time.Millisecond))
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-Manager.stopCh:
			return false
		case <-timer.C:
			return true
		}
	}
	// 共享账号池（并发安全），goroutine 动态领取账号（仅 Outlook 模式使用）
	var accountPoolMu sync.Mutex
	accountPoolIdx := 0
	nextAccount := func() (email.OutlookAccount, bool) {
		accountPoolMu.Lock()
		defer accountPoolMu.Unlock()
		if accountPoolIdx >= len(outlookAccounts) {
			return email.OutlookAccount{}, false
		}
		acc := outlookAccounts[accountPoolIdx]
		accountPoolIdx++
		return acc, true
	}

	// MoeMail 域名池索引（并发安全）
	var moemailDomainIdx int
	var moemailDomainMu sync.Mutex
	nextMoeMailDomain := func() (string, email.MoeMailConfig) {
		moemailDomainMu.Lock()
		defer moemailDomainMu.Unlock()

		var domain string
		if req.MoeMailRandomMode {
			domain = moemailDomainPool[rand.Intn(len(moemailDomainPool))]
		} else {
			domain = moemailDomainPool[moemailDomainIdx%len(moemailDomainPool)]
			moemailDomainIdx++
		}

		configs := moemailDomainConfigs[domain]
		return domain, configs[rand.Intn(len(configs))]
	}

	// CloudMail 域名池索引（并发安全）
	var cloudmailDomainIdx int
	var cloudmailDomainMu sync.Mutex
	nextCloudMailDomain := func() (string, email.CloudMailConfig) {
		cloudmailDomainMu.Lock()
		defer cloudmailDomainMu.Unlock()

		var domain string
		if req.CloudMailRandomMode {
			domain = cloudmailDomainPool[rand.Intn(len(cloudmailDomainPool))]
		} else {
			domain = cloudmailDomainPool[cloudmailDomainIdx%len(cloudmailDomainPool)]
			cloudmailDomainIdx++
		}

		configs := cloudmailDomainConfigs[domain]
		return domain, configs[rand.Intn(len(configs))]
	}

	var reusableEmails reusableEmailPool
	graphProfileResolver := newCachedOutlookGraphProfileResolver(func(acc email.OutlookAccount, proxyURL string) (email.OutlookGraphProfile, error) {
		profile, err := email.GetOutlookGraphProfileWithProxy(acc, proxyURL)
		if err != nil {
			statsMu.Lock()
			runtimeStats.RecordGraphFailure(classifyGraphResolutionError(err.Error()))
			statsMu.Unlock()
			publishDiagnostics()
		}
		return profile, err
	})
	registrationTracker := newOutlookRegistrationAddressTracker()

	// requestTaskStop 在内部达成停止条件时关闭任务信号，避免继续领取新尝试。
	var stopOnce sync.Once
	requestTaskStop := func(message string) {
		stopOnce.Do(func() {
			if strings.TrimSpace(message) != "" {
				log.Println(message)
			}
			Manager.mu.Lock()
			select {
			case <-Manager.stopCh:
			default:
				close(Manager.stopCh)
			}
			Manager.mu.Unlock()
			taskCancel()
		})
	}

	// 成功目标模式下用 in-flight 计数控制新尝试，避免并发超过剩余成功名额。
	var attemptMu sync.Mutex
	nextAttemptIndex := 0
	inFlightAttempts := 0
	nextSuccessTargetAttempt := func() (int, bool) {
		attemptMu.Lock()
		defer attemptMu.Unlock()
		select {
		case <-Manager.stopCh:
			return 0, false
		default:
		}
		Manager.mu.Lock()
		currentSuccess := Manager.success
		Manager.mu.Unlock()
		if currentSuccess >= successTarget || currentSuccess+inFlightAttempts >= successTarget {
			return 0, false
		}
		idx := nextAttemptIndex
		nextAttemptIndex++
		inFlightAttempts++
		return idx, true
	}
	finishSuccessTargetAttempt := func() {
		attemptMu.Lock()
		if inFlightAttempts > 0 {
			inFlightAttempts--
		}
		attemptMu.Unlock()
	}
	registrationCounter := newRegistrationAttemptCounter(req.Count)
	reserveRegistrationAttempt := func(candidateIndex int) (int, bool) {
		if successTargetMode {
			return candidateIndex, true
		}
		return registrationCounter.reserve()
	}
	var preparationMu sync.Mutex
	nextPreparationIndex := 0
	nextPreparationAttempt := func() (int, bool) {
		select {
		case <-Manager.stopCh:
			return 0, false
		default:
		}
		if !successTargetMode && registrationCounter.done() {
			return 0, false
		}
		preparationMu.Lock()
		idx := nextPreparationIndex
		nextPreparationIndex++
		preparationMu.Unlock()
		return idx, true
	}

	// send-otp 400 熔断：任一任务遇到该错误即终止全部并发任务（只触发一次）
	var otpKillOnce sync.Once
	postPasswordSuspendedRegions := make(map[string]int)
	var postPasswordSuspendedRegionsMu sync.Mutex
	runtimeProxyEgressStartPacer := newRuntimeProxyEgressStartPacer(runtimeProxyCountryStartPacingStep)
	doTask := func(i int) {
		select {
		case <-Manager.stopCh:
			return
		default:
		}

		taskCfg := *taskConfig
		defer closeTaskConfigIdleConnections(&taskCfg)
		emailProvider, selectorErr := providerSelector.NextWithContext(taskCtx)
		if selectorErr != nil {
			log.Printf("[Kiro][%d/%d] 选择邮箱渠道失败: %v", i+1, displayTotal, selectorErr)
			return
		}
		taskCfg.EmailProvider = emailProvider
		taskCfg.UseOutlook = emailProvider == "outlook"
		taskCfg.UseMoeMail = emailProvider == "moemail"
		taskCfg.UseCloudMail = emailProvider == "cloudmail"
		taskCfg.Password = core.GenPassword()
		log.Printf("[Kiro][%d/%d] 本次邮箱渠道: %s", i+1, displayTotal, emailProviderDisplayName(emailProvider))
		var accountEmail string
		if proxyMode == storage.ProxyModePool {
			// 多代理池作为独立模式：仅在 pool 模式下按权重为本次注册选择代理。
			if picked := proxy.PickRandom(); picked != "" {
				taskCfg.Proxy = picked
				log.Printf("[Kiro][%d/%d] 选中代理池代理 %s", i+1, displayTotal, proxy.MaskURL(picked))
			}
		}
		var currentEmail string
		setOutlookAccount := func(acc email.OutlookAccount) bool {
			if taskCfg.UseOutlookGraph() {
				graphMode := storage.GetOutlookGraphRegistrationEmailMode()
				resolved, graphErr := resolveOutlookGraphRegistrationEmailForTask(acc, taskCfg.EmailProxy, graphMode, graphProfileResolver)
				acc = resolved
				if graphErr != nil && graphMode != storage.OutlookGraphRegistrationEmailImported {
					failReason := classifyGraphResolutionError(graphErr.Error())
					log.Printf("[Kiro][%d/%d] Outlook Graph 地址解析失败，跳过账号: %s (%s: %v)", i+1, displayTotal, acc.Email, failReason, graphErr)
					if shouldConsumeOutlookAccountAfterGraphResolutionFailure(failReason) {
						email.UpdateAccountStatus(acc.Email, true, false, failReason)
					} else {
						email.MarkAccountFailReason(acc.Email, failReason)
					}
					Manager.mu.Lock()
					completedCount, successCount, failedCount := Manager.completed, Manager.success, Manager.failed
					Manager.mu.Unlock()
					statsMu.Lock()
					progressSummary := runtimeStats.ProgressSummary(completedCount, successCount, failedCount, 3)
					statsMu.Unlock()
					publishDiagnostics()
					log.Println(progressSummary)
					return false
				}
				hasGraphResolution := strings.TrimSpace(acc.GraphPrimaryEmail) != "" || strings.TrimSpace(acc.GraphResolvedAt) != ""
				if graphMode != storage.OutlookGraphRegistrationEmailImported && strings.TrimSpace(acc.RegistrationEmail) != "" && hasGraphResolution {
					email.SaveOutlookGraphResolution(acc.Email, acc)
				}
			}
			if registrationTracker.ShouldSkip(acc) {
				log.Printf("[Kiro][%d/%d] 跳过重复/已注册最终邮箱: %s -> %s", i+1, displayTotal, acc.Email, strings.TrimSpace(acc.RegistrationEmail))
				email.UpdateAccountStatus(acc.Email, true, false, "邮箱已注册")
				Manager.mu.Lock()
				completedCount, successCount, failedCount := Manager.completed, Manager.success, Manager.failed
				Manager.mu.Unlock()
				statsMu.Lock()
				runtimeStats.RecordRegisteredSkip()
				progressSummary := runtimeStats.ProgressSummary(completedCount, successCount, failedCount, 3)
				statsMu.Unlock()
				publishDiagnostics()
				log.Println(progressSummary)
				return false
			}
			registrationTracker.MarkAttempt(acc)
			taskCfg.OutlookAccount = &acc
			accountEmail = acc.Email
			currentEmail = acc.Email
			return true
		}
		recordEmailCreateFailure := func(providerLabel string, err error) {
			Manager.mu.Lock()
			completedCount := Manager.completed
			successCount := Manager.success
			failedCount := Manager.failed
			Manager.mu.Unlock()
			log.Printf("[Kiro][%d/%d] 生成 %s 邮箱失败（前置失败，不计入注册次数，继续补齐）: %v", i+1, displayTotal, providerLabel, err)
			if err == nil {
				return
			}
			statsMu.Lock()
			runtimeStats.RecordPreflightEmailCreateFailure(err.Error())
			progressSummary := runtimeStats.ProgressSummary(completedCount, successCount, failedCount, 3)
			statsMu.Unlock()
			publishDiagnostics()
			log.Println(progressSummary)
			errorMsg := err.Error()
			if !isSuccessfulDomainPreferenceMiss(errorMsg) && providerSelector.ReportCreateFailure(emailProvider) {
				log.Printf("[Kiro] %s 邮箱创建连续失败 %d 次，冷却 %s 后再试", providerLabel, emailProviderCreateFailureCooldownThreshold, emailProviderCreateFailureCooldown)
			}
			if shouldStopTaskForEmailCreateFailure(errorMsg) {
				requestTaskStop(fmt.Sprintf("[Kiro] %s 邮箱服务不可用，停止任务；请更换邮箱代理/渠道", providerLabel))
				return
			}
			if isEmailProviderAccessBlockedError(errorMsg) {
				log.Printf("[Kiro] %s 邮箱服务拒绝当前出口国家/IP，本次尝试失败，批次继续；请更换邮箱代理/Clash 节点或邮箱渠道", providerLabel)
				return
			}
			if isEmailProviderRateLimitError(errorMsg) {
				log.Printf("[Kiro] %s 邮箱服务限流，已重试 %d 次仍失败；本次尝试失败，批次继续；请稍后重试或更换邮箱代理/渠道", providerLabel, len(emailProviderRateLimitBackoffs))
			}
		}
		recordProxySelectFailure := func(err error) {
			Manager.mu.Lock()
			completedCount := Manager.completed
			successCount := Manager.success
			failedCount := Manager.failed
			Manager.mu.Unlock()
			log.Printf("[Kiro][%d/%d] 代理预选失败（前置失败，不计入注册次数，继续补齐）: %v", i+1, displayTotal, err)
			if err == nil {
				return
			}
			statsMu.Lock()
			runtimeStats.RecordPreflightProxySelectFailure(err.Error())
			progressSummary := runtimeStats.ProgressSummary(completedCount, successCount, failedCount, 3)
			statsMu.Unlock()
			publishDiagnostics()
			log.Println(progressSummary)
		}
		createTempEmailWithRetry := func(providerLabel string, create func() (string, error)) (string, error) {
			return createTempEmailWithRateLimitRetry(taskCtx, providerLabel, i+1, displayTotal, create)
		}
		nextUsableOutlookAccount := func() bool {
			for {
				acc, ok := nextAccount()
				if !ok {
					return false
				}
				if setOutlookAccount(acc) {
					return true
				}
			}
		}

		reusedEmail := false
		if candidate, ok := takeReusableFailedEmail(req, &reusableEmails, emailProvider); ok {
			if address, applied := applyReusableEmailCandidate(emailProvider, &taskCfg, candidate); applied {
				currentEmail = address
				reusedEmail = true
				log.Printf("[Kiro][%d/%d] 复用临时邮箱: %s", i+1, displayTotal, currentEmail)
			}
		}

		// 根据邮箱提供商类型获取邮箱
		if emailProvider == "outlook" {
			// Outlook 模式：从共享池领取账号
			if !nextUsableOutlookAccount() {
				log.Printf("[Kiro][%d/%d] 无可用账号，跳过", i+1, displayTotal)
				recordEmailCreateFailure("Outlook", fmt.Errorf("无可用账号"))
				if successTargetMode && onlyOutlookProvider {
					requestTaskStop("[Kiro] Outlook 账号池已耗尽，停止成功目标模式")
				} else if !successTargetMode && onlyOutlookProvider {
					requestTaskStop("[Kiro] Outlook 账号池已耗尽，停止任务")
				}
				return
			}
		} else if emailProvider == "moemail" {
			if reusedEmail {
				currentEmail = taskCfg.MoeMailProvider.GetAddress()
			} else {
				// MoeMail 模式：动态生成临时邮箱
				// 从域名池中获取域名和配置
				domain, config := nextMoeMailDomain()

				// 生成完全随机的邮箱名
				emailName := email.GenerateEmailName(i)

				// 使用 1 小时有效期
				expiryTime := int64(3600000) // 1 小时（毫秒）

				log.Printf("[Kiro][%d/%d] 创建 MoeMail 邮箱: %s@%s (配置: %s)", i+1, displayTotal, emailName, domain, config.Name)

				// 创建 MoeMail 提供商
				provider, err := email.NewMoeMailProviderWithProxy(config, emailName, expiryTime, domain, taskCfg.EmailProxy)
				if err != nil {
					recordEmailCreateFailure("MoeMail", err)
					return
				}

				taskCfg.MoeMailProvider = provider
				currentEmail = provider.GetAddress()
			}
		} else if emailProvider == "cloudmail" {
			if reusedEmail {
				currentEmail = taskCfg.CloudMailProvider.GetAddress()
			} else {
				domain, config := nextCloudMailDomain()
				emailName := email.GenerateEmailName(i)

				log.Printf("[Kiro][%d/%d] 创建 cloud-mail 邮箱: %s@%s (配置: %s)", i+1, displayTotal, emailName, domain, config.Name)

				provider, err := email.NewCloudMailProvider(config, emailName, domain)
				if err != nil {
					recordEmailCreateFailure("cloud-mail", err)
					return
				}

				taskCfg.CloudMailProvider = provider
				cfgCopy := config
				taskCfg.CloudMailConfig = &cfgCopy
				currentEmail = provider.GetAddress()
			}
		} else if emailProvider == "mailporary" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 Mailporary 邮箱", i+1, displayTotal)
				service := email.NewMailporaryService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("Mailporary", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("Mailporary", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "emailnator" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 Emailnator 邮箱", i+1, displayTotal)
				service := email.NewEmailnatorService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("Emailnator", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("Emailnator", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "mailgw" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 mail.gw 邮箱", i+1, displayTotal)
				service := email.NewMailGWService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("mail.gw", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("mail.gw", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "mailtm" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 mail.tm 邮箱", i+1, displayTotal)
				service := email.NewMailTMService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("mail.tm", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("mail.tm", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "tempmail_lol" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 TempMail.lol 邮箱", i+1, displayTotal)
				service := email.NewTempMailLOLService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("TempMail.lol", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("TempMail.lol", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "guerrillamail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 GuerrillaMail 邮箱", i+1, displayTotal)
				service := email.NewGuerrillaMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("GuerrillaMail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("GuerrillaMail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "mailtemp" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 MailTemp 邮箱", i+1, displayTotal)
				service := email.NewMailTempService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("MailTemp", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("MailTemp", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "tempmail_plus" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 TempMail.plus 邮箱", i+1, displayTotal)
				service := email.NewTempMailPlusService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("TempMail.plus", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("TempMail.plus", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "inboxkitten" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 InboxKitten 邮箱", i+1, displayTotal)
				service := email.NewInboxKittenService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("InboxKitten", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("InboxKitten", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "inboxes" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 Inboxes 邮箱", i+1, displayTotal)
				service := email.NewInboxesService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("Inboxes", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("Inboxes", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "freecustom" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 FreeCustom.Email 邮箱", i+1, displayTotal)
				service := email.NewFreeCustomService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("FreeCustom.Email", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("FreeCustom.Email", err)
					return
				}
				currentEmail = address
			}
		} else if channel, ok := email.FreeCustomFixedDomainChannel(emailProvider); ok {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 %s 邮箱", i+1, displayTotal, channel.Label)
				service := email.NewFreeCustomFixedDomainService(taskCfg.EmailProxy, channel.Domain)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry(channel.Label, service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure(channel.Label, err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "dropmail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 DropMail 邮箱", i+1, displayTotal)
				service := email.NewDropMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("DropMail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("DropMail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "mailcatch" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 MailCatch 邮箱", i+1, displayTotal)
				service := email.NewMailCatchService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("MailCatch", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("MailCatch", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "tempmailo" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 TempMailo 邮箱", i+1, displayTotal)
				service := email.NewTempMailoService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("TempMailo", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("TempMailo", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "smailpro" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 SmailPro 邮箱", i+1, displayTotal)
				service, address, err := createTempEmailPreferringSuccessfulDomains(taskCtx, "smailpro", "SmailPro", i+1, displayTotal, req.DomainExplorationPercent, func() (email.TempEmailService, string, error) {
					service := email.NewSmailProService(taskCfg.EmailProxy)
					taskCfg.TempEmailService = service
					address, err := createTempEmailWithRetry("SmailPro", service.CreateWithError)
					return service, address, err
				})
				if err != nil {
					recordEmailCreateFailure("SmailPro", err)
					return
				}
				taskCfg.TempEmailService = service
				currentEmail = address
			}
		} else if emailProvider == "tempmailbox" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 TempMailbox 邮箱", i+1, displayTotal)
				service := email.NewTempMailboxService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("TempMailbox", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("TempMailbox", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "minuteinbox" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 MinuteInbox 邮箱", i+1, displayTotal)
				service := email.NewMinuteInboxService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("MinuteInbox", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("MinuteInbox", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "generator_email" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 Generator.Email 邮箱", i+1, displayTotal)
				service := email.NewGeneratorEmailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("Generator.Email", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("Generator.Email", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "mailtowin" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 MailToWin 邮箱", i+1, displayTotal)
				service := email.NewMailToWinService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("MailToWin", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("MailToWin", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "mail2me" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 Mail2Me 邮箱", i+1, displayTotal)
				service := email.NewMail2MeService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("Mail2Me", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("Mail2Me", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "pickmemail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 PickMeMail 邮箱", i+1, displayTotal)
				service := email.NewPickMeMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("PickMeMail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("PickMeMail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "maximail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 MaxiMail 邮箱", i+1, displayTotal)
				service := email.NewMaxiMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("MaxiMail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("MaxiMail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "emlpro" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 EmlPro 邮箱", i+1, displayTotal)
				service := email.NewEmlProService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("EmlPro", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("EmlPro", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "freeml" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 FreeML 邮箱", i+1, displayTotal)
				service := email.NewFreeMLService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("FreeML", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("FreeML", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "emlhub" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 EmlHub 邮箱", i+1, displayTotal)
				service := email.NewEmlHubService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("EmlHub", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("EmlHub", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "emltmp" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 EmlTmp 邮箱", i+1, displayTotal)
				service := email.NewEmlTmpService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("EmlTmp", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("EmlTmp", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "mailpwr" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 MailPwr 邮箱", i+1, displayTotal)
				service := email.NewMailPwrService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("MailPwr", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("MailPwr", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "tenmail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 10Mail 邮箱", i+1, displayTotal)
				service := email.NewTenMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("10Mail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("10Mail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "dropmail_me" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 DropMail.me 邮箱", i+1, displayTotal)
				service := email.NewDropMailMeService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("DropMail.me", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("DropMail.me", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "mimimail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 MimiMail 邮箱", i+1, displayTotal)
				service := email.NewMimiMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("MimiMail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("MimiMail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "pickmail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 PickMail 邮箱", i+1, displayTotal)
				service := email.NewPickMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("PickMail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("PickMail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "spymail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 SpyMail 邮箱", i+1, displayTotal)
				service := email.NewSpyMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("SpyMail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("SpyMail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "yomail" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 YoMail 邮箱", i+1, displayTotal)
				service := email.NewYoMailService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("YoMail", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("YoMail", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "tmio_bltiwd" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 TempMailIO bltiwd.com 邮箱", i+1, displayTotal)
				service := email.NewTempMailIOBltiwdService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("TempMailIO bltiwd.com", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("TempMailIO bltiwd.com", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "tmio_wnbaldwy" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 TempMailIO wnbaldwy.com 邮箱", i+1, displayTotal)
				service := email.NewTempMailIOWnbaldwyService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("TempMailIO wnbaldwy.com", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("TempMailIO wnbaldwy.com", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "tmio_bwmyga" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 TempMailIO bwmyga.com 邮箱", i+1, displayTotal)
				service := email.NewTempMailIOBwmygaService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("TempMailIO bwmyga.com", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("TempMailIO bwmyga.com", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "tmio_ozsaip" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 TempMailIO ozsaip.com 邮箱", i+1, displayTotal)
				service := email.NewTempMailIOOzsaipService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("TempMailIO ozsaip.com", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("TempMailIO ozsaip.com", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "gonebox" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 GoneBox 邮箱", i+1, displayTotal)
				service := email.NewGoneBoxService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("GoneBox", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("GoneBox", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "openinbox" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 OpenInbox 邮箱", i+1, displayTotal)
				service := email.NewOpenInboxService(taskCfg.EmailProxy)
				taskCfg.TempEmailService = service
				address, err := createTempEmailWithRetry("OpenInbox", service.CreateWithError)
				if err != nil {
					recordEmailCreateFailure("OpenInbox", err)
					return
				}
				currentEmail = address
			}
		} else if emailProvider == "blinkbox" {
			if reusedEmail {
				currentEmail = taskCfg.TempEmailService.GetAddress()
			} else {
				log.Printf("[Kiro][%d/%d] 创建 BlinkBoxApp 邮箱", i+1, displayTotal)
				service, address, err := createTempEmailPreferringSuccessfulDomains(taskCtx, "blinkbox", "BlinkBoxApp", i+1, displayTotal, req.DomainExplorationPercent, func() (email.TempEmailService, string, error) {
					service := email.NewBlinkBoxService(taskCfg.EmailProxy)
					taskCfg.TempEmailService = service
					address, err := createTempEmailWithRetry("BlinkBoxApp", service.CreateWithError)
					return service, address, err
				})
				if err != nil {
					recordEmailCreateFailure("BlinkBoxApp", err)
					return
				}
				taskCfg.TempEmailService = service
				currentEmail = address
			}
		}

		if currentEmail != "" && emailProvider != "outlook" && !reusedEmail {
			providerSelector.ReportCreateSuccess(emailProvider)
		}

		var itemStart time.Time
		var result map[string]interface{}
		registrationReserved := false
		domainAttemptRecorded := false
		reserveBeforeRegistrar := func() bool {
			if registrationReserved {
				return true
			}
			registrationIndex, reserved := reserveRegistrationAttempt(i)
			if !reserved {
				log.Printf("[Kiro][%d/%d] 注册次数已满，丢弃已创建邮箱并停止领取新尝试", i+1, displayTotal)
				return false
			}
			i = registrationIndex
			registrationReserved = true
			if !waitForRegistrarStartPacing(i, displayTotal) {
				result = map[string]interface{}{
					"status": "failed",
					"error":  "任务已取消",
					"email":  currentEmail,
				}
				return false
			}
			itemStart = time.Now()
			log.Printf("[Kiro][%d/%d] 开始注册", i+1, displayTotal)
			if !domainAttemptRecorded {
				recordEmailProviderDomainAttemptStat(emailProvider, currentEmail)
				domainAttemptRecorded = true
			}
			return true
		}
		releaseRegistrationReservation := func() {
			if !registrationReserved || successTargetMode {
				return
			}
			registrationCounter.release()
			registrationReserved = false
		}

		maxAttempts := req.RetryCount + 1
		const maxProxySwitches = 3

		proxySwitches := 0
		currentClashNode := ""
		currentClashAssisted := false
		currentClashFingerprintPrefix := ""
		currentPoolProxy := ""
		currentRuntimeProxySourceKey := ""
		currentRuntimeProxyEgress := runtimeProxyEgressInfo{}
	retryLoop:
		for attempt := 0; attempt < maxAttempts; attempt++ {
			// 每次重试前检查停止信号
			select {
			case <-Manager.stopCh:
				result = map[string]interface{}{
					"status": "failed",
					"error":  "任务已取消",
					"email":  currentEmail,
				}
				break retryLoop
			default:
			}

			if attempt > 0 {
				log.Printf("[Kiro][%d/%d] 第 %d 次重试", i+1, displayTotal, attempt)
				select {
				case <-Manager.stopCh:
					result = map[string]interface{}{
						"status": "failed",
						"error":  "任务已取消",
						"email":  currentEmail,
					}
					break retryLoop
				case <-time.After(time.Duration(2+attempt) * time.Second):
				}
			}

			if taskCtx.Err() != nil {
				result = map[string]interface{}{
					"status": "failed",
					"error":  "任务已取消",
					"email":  currentEmail,
				}
				break retryLoop
			}

			attemptCfg := taskCfg
			runRegistrar := func(cfg *core.Config) map[string]interface{} {
				reg := core.NewRegistrar(cfg)
				defer reg.CloseIdleConnections()
				reg.Ctx = taskCtx
				reg.TaskLabel = fmt.Sprintf("%d/%d", i+1, displayTotal)
				return reg.Run()
			}
			runAttempt := func() bool {
				currentClashAssisted = false
				currentRuntimeProxySourceKey = ""
				currentRuntimeProxyEgress = runtimeProxyEgressInfo{}
				if clashEnabled {
					clashMu.Lock()
					defer clashMu.Unlock()

					selection, err := clashClient.SwitchToNextAvailable(taskCtx)
					if err != nil {
						log.Printf("[Kiro][%d/%d] Clash 节点选择失败: %v", i+1, displayTotal, err)
						result = map[string]interface{}{
							"status": "failed",
							"error":  "Clash 无可用节点: " + err.Error(),
							"email":  currentEmail,
						}
						return false
					}
					locale := applyClashSelectionToConfigForSubject(&attemptCfg, taskConfig.Proxy, selection, clashFingerprintPrefix, fingerprintSubjectForTask(&attemptCfg, currentEmail))
					currentClashNode = selection.Node
					currentClashAssisted = true
					currentClashFingerprintPrefix = clashFingerprintPrefix
					delayText := "跳过连通性测试"
					if !selection.SkippedTest {
						delayText = fmt.Sprintf("延迟 %dms", selection.DelayMs)
					}
					log.Printf("[Kiro][%d/%d] Clash 节点已绑定本次注册: %s / %s (%s, 尝试 %d 个, 耗时 %dms)",
						i+1, displayTotal, selection.ProxyGroup, selection.Node, delayText, selection.Attempts, selection.DurationMs)
					log.Printf("[Kiro][%d/%d] 浏览器地区已绑定: acceptLanguage=%s, i18next=%s, timeZone=%d",
						i+1, displayTotal, locale.AcceptLanguage, locale.I18Next, locale.TimeZone)

					if !reserveBeforeRegistrar() {
						return false
					}
					result = runRegistrar(&attemptCfg)
					return true
				}

				if normalClashAssist {
					clashMu.Lock()
					selection, err := clashClient.SwitchToNextAvailable(taskCtx)
					clashMu.Unlock()
					if err != nil {
						log.Printf("[Kiro][%d/%d] 普通代理 Clash 辅助真实性不可用，降级为普通代理: %v", i+1, displayTotal, err)
						normalClashAssist = false
					} else {
						locale := applyClashSelectionToConfigForSubject(&attemptCfg, taskConfig.Proxy, selection, normalClashFingerprintPrefix, fingerprintSubjectForTask(&attemptCfg, currentEmail))
						currentClashNode = selection.Node
						currentClashAssisted = true
						currentClashFingerprintPrefix = normalClashFingerprintPrefix
						delayText := "跳过连通性测试"
						if !selection.SkippedTest {
							delayText = fmt.Sprintf("延迟 %dms", selection.DelayMs)
						}
						log.Printf("[Kiro][%d/%d] 普通代理已绑定 Clash 节点: %s / %s (%s, 尝试 %d 个, 耗时 %dms)",
							i+1, displayTotal, selection.ProxyGroup, selection.Node, delayText, selection.Attempts, selection.DurationMs)
						log.Printf("[Kiro][%d/%d] 浏览器地区已绑定: acceptLanguage=%s, i18next=%s, timeZone=%d",
							i+1, displayTotal, locale.AcceptLanguage, locale.I18Next, locale.TimeZone)
					}
				}

				if attemptCfg.Proxy == "" {
					if !reserveBeforeRegistrar() {
						return false
					}
					result = runRegistrar(&attemptCfg)
					return true
				}

				var (
					selection               proxy.Selection
					egress                  runtimeProxyEgressInfo
					preferredProxyEgressHit bool
				)
				if !currentClashAssisted && proxyMode == storage.ProxyModeNormal && proxy.HasURLTemplate(attemptCfg.Proxy) {
					currentRuntimeProxySourceKey = runtimeProxySourceKey(attemptCfg.Proxy)
					selection, egress, preferredProxyEgressHit, err = selectRuntimeProxyWithStoredEgressPolicy(
						taskCtx,
						attemptCfg.Proxy,
						proxy.DefaultRegisterSelectOptions(),
						detectRuntimeProxyEgressInfo,
						runtimeProxyEgressMaxAttempts,
						req.ProxyExplorationPercent,
					)
				} else {
					if !currentClashAssisted && proxyMode == storage.ProxyModeNormal && strings.TrimSpace(attemptCfg.Proxy) != "" {
						currentRuntimeProxySourceKey = runtimeProxySourceKey(attemptCfg.Proxy)
					}
					selection, err = proxy.SelectRuntimeProxy(taskCtx, attemptCfg.Proxy, proxy.DefaultRegisterSelectOptions())
				}
				if err != nil {
					log.Printf("[Kiro][%d/%d] 代理候选选择失败: %v", i+1, displayTotal, err)
					if !registrationReserved {
						recordProxySelectFailure(err)
						return false
					}
					errorPrefix := "代理无可用节点"
					if proxyMode == storage.ProxyModePool {
						errorPrefix = "代理池无可用节点"
					}
					result = map[string]interface{}{
						"status": "failed",
						"error":  errorPrefix + ": " + err.Error(),
						"email":  currentEmail,
					}
					return false
				}
				attemptCfg.Proxy = selection.ProxyURL
				if selection.Templated && strings.TrimSpace(attemptCfg.FingerprintKey) == "" {
					attemptCfg.FingerprintKey = normalTemplateFingerprintKey(selection.ProxyURL, fingerprintSubjectForTask(&attemptCfg, currentEmail))
				}
				if proxyMode == storage.ProxyModePool {
					currentPoolProxy = selection.ProxyURL
				}
				attemptCfg.ProxyFromPool = proxyMode == storage.ProxyModePool
				attemptCfg.ProxySwitchable = selection.Templated
				if selection.Templated {
					if proxyMode == storage.ProxyModePool {
						log.Printf("[Kiro][%d/%d] 代理池候选可用: 第 %d/%d 个, 耗时 %dms, %s",
							i+1, displayTotal, selection.SuccessAttempt, selection.Attempts, selection.Duration.Milliseconds(), selection.MaskedProxyURL)
					} else {
						log.Printf("[Kiro][%d/%d] 代理模板候选可用: 第 %d/%d 个, 耗时 %dms, %s",
							i+1, displayTotal, selection.SuccessAttempt, selection.Attempts, selection.Duration.Milliseconds(), selection.MaskedProxyURL)
					}
				} else {
					log.Printf("[Kiro][%d/%d] 代理验证可用: %s", i+1, displayTotal, selection.MaskedProxyURL)
				}
				if !currentClashAssisted {
					var geoErr error
					if egress.IP == "" {
						egress, geoErr = detectRuntimeProxyEgressInfo(taskCtx, attemptCfg.Proxy)
						egress = normalizeRuntimeProxyEgressInfo(egress)
					} else if preferredProxyEgressHit {
						log.Printf("[Kiro][%d/%d] 运行时代理命中历史成功出口 IP: ip=%s country=%s", i+1, displayTotal, egress.IP, egress.CountryCode)
					} else if proxyMode == storage.ProxyModeNormal && selection.Templated {
						log.Printf("[Kiro][%d/%d] 运行时代理使用探索/回退出口 IP: ip=%s country=%s", i+1, displayTotal, egress.IP, egress.CountryCode)
					}
					if geoErr != nil {
						log.Printf("[Kiro][%d/%d] 运行时代理出口探测失败，保留默认浏览器地区: %v", i+1, displayTotal, geoErr)
					} else if locale, ok := applyRuntimeProxyCountryLocaleToConfigForSubject(&attemptCfg, egress.CountryCode, fingerprintSubjectForTask(&attemptCfg, currentEmail)); ok {
						currentRuntimeProxyEgress = egress
						log.Printf("[Kiro][%d/%d] 运行时代理出口地区已绑定: country=%s acceptLanguage=%s, i18next=%s, timeZone=%d",
							i+1, displayTotal, egress.CountryCode, locale.AcceptLanguage, locale.I18Next, locale.TimeZone)
					} else {
						currentRuntimeProxyEgress = egress
						if egress.CountryCode == "" {
							log.Printf("[Kiro][%d/%d] 运行时代理出口 IP 已识别但无地区信息: ip=%s，保留默认浏览器地区", i+1, displayTotal, egress.IP)
						} else {
							log.Printf("[Kiro][%d/%d] 运行时代理出口地区 %s 暂无地区指纹映射，保留默认浏览器地区", i+1, displayTotal, egress.CountryCode)
						}
					}
				}
				if currentRuntimeProxyEgress.IP != "" {
					delay := runtimeProxyEgressStartPacer.ReserveDelay(currentRuntimeProxyEgress.IP, time.Now())
					if delay > 0 {
						log.Printf("[Kiro][%d/%d] 同出口 IP 真实注册启动节流: ip=%s, 等待 %ds，避免短时间并发触发风控",
							i+1, displayTotal, currentRuntimeProxyEgress.IP, int(delay.Seconds()))
						timer := time.NewTimer(delay)
						select {
						case <-taskCtx.Done():
							if !timer.Stop() {
								select {
								case <-timer.C:
								default:
								}
							}
							result = map[string]interface{}{
								"status": "failed",
								"error":  "任务已取消",
								"email":  currentEmail,
							}
							return false
						case <-timer.C:
						}
					}
				}
				if !reserveBeforeRegistrar() {
					return false
				}
				if !currentClashAssisted && proxyMode == storage.ProxyModeNormal && currentRuntimeProxySourceKey != "" && currentRuntimeProxyEgress.IP != "" {
					if err := recordRuntimeProxyEgressAttempt(currentRuntimeProxySourceKey, currentRuntimeProxyEgress); err != nil {
						log.Printf("[Kiro][%d/%d] 记录代理出口尝试统计失败: ip=%s err=%v", i+1, displayTotal, currentRuntimeProxyEgress.IP, err)
					}
				}
				result = runRegistrar(&attemptCfg)
				return true
			}
			if ok := runAttempt(); !ok {
				if !registrationReserved {
					return
				}
				break
			}
			if resultEmail, _ := result["email"].(string); resultEmail != "" {
				currentEmail = resultEmail
			}
			recordEmailProviderOTPStat(emailProvider, result)

			if result["status"] == "success" {
				if currentClashAssisted && currentClashNode != "" {
					clashMu.Lock()
					clashClient.RecordNodeSuccess(currentClashNode)
					clashMu.Unlock()
					postPasswordSuspendedRegionsMu.Lock()
					resetPostPasswordSuspendedRegion(postPasswordSuspendedRegions, currentClashNode)
					postPasswordSuspendedRegionsMu.Unlock()
				}
				if proxyMode == storage.ProxyModePool && currentPoolProxy != "" {
					proxy.RecordPoolProxySuccess(currentPoolProxy)
				}
				if !currentClashAssisted && proxyMode == storage.ProxyModeNormal && currentRuntimeProxySourceKey != "" && currentRuntimeProxyEgress.IP != "" {
					if err := recordRuntimeProxyEgressSuccess(currentRuntimeProxySourceKey, currentRuntimeProxyEgress); err != nil {
						log.Printf("[Kiro][%d/%d] 记录代理出口成功统计失败: ip=%s err=%v", i+1, displayTotal, currentRuntimeProxyEgress.IP, err)
					}
				}
				break
			}

			errorMsg, _ := result["error"].(string)
			proxyNetworkErr := isProxyNetworkError(errorMsg)
			clashRiskErr := isClashNodeRiskFailure(errorMsg)
			if !currentClashAssisted && proxyMode == storage.ProxyModeNormal && currentRuntimeProxySourceKey != "" && currentRuntimeProxyEgress.IP != "" {
				if isRuntimeProxyCountryRiskFailure(errorMsg) {
					if err := recordRuntimeProxyEgressRiskFailure(currentRuntimeProxySourceKey, currentRuntimeProxyEgress); err != nil {
						log.Printf("[Kiro][%d/%d] 记录代理出口风控统计失败: ip=%s err=%v", i+1, displayTotal, currentRuntimeProxyEgress.IP, err)
					} else {
						log.Printf("[Kiro][%d/%d] 运行时代理出口 IP 触发注册/验活风控，短期冷却该出口 IP: ip=%s country=%s (%s)",
							i+1, displayTotal, currentRuntimeProxyEgress.IP, currentRuntimeProxyEgress.CountryCode, errorMsg)
					}
				} else if proxyNetworkErr {
					if err := recordRuntimeProxyEgressNetworkFailure(currentRuntimeProxySourceKey, currentRuntimeProxyEgress); err != nil {
						log.Printf("[Kiro][%d/%d] 记录代理出口网络失败统计失败: ip=%s err=%v", i+1, displayTotal, currentRuntimeProxyEgress.IP, err)
					}
				}
			}
			if currentClashAssisted && currentClashNode != "" {
				switch {
				case proxyNetworkErr:
					statsMu.Lock()
					runtimeStats.clashNetworkErrors++
					statsMu.Unlock()
					clashMu.Lock()
					clashClient.RecordNodeNetworkFailure(currentClashNode)
					clashMu.Unlock()
				case clashRiskErr:
					statsMu.Lock()
					runtimeStats.clashRiskFailures++
					statsMu.Unlock()
					clashMu.Lock()
					clashClient.RecordNodeRiskFailure(currentClashNode)
					clashMu.Unlock()
					log.Printf("[Kiro][%d/%d] Clash 节点触发注册/验活风控，临时隔离: %s (%s)", i+1, displayTotal, currentClashNode, errorMsg)
				default:
					clashMu.Lock()
					clashClient.RecordNodeSuccess(currentClashNode)
					clashMu.Unlock()
				}
			}
			if proxyMode == storage.ProxyModePool && currentPoolProxy != "" {
				if proxyNetworkErr {
					statsMu.Lock()
					runtimeStats.poolNetworkErrors++
					statsMu.Unlock()
					proxy.RecordPoolProxyNetworkFailure(currentPoolProxy)
				} else {
					proxy.RecordPoolProxySuccess(currentPoolProxy)
				}
			}

			if currentClashAssisted && isModelVerificationAccessDeniedError(errorMsg) && !shouldRotateOutlookAfterPostPasswordModelFailure(taskCfg.UseOutlook, result) && proxySwitches < maxProxySwitches {
				if currentClashFingerprintPrefix == "" {
					currentClashFingerprintPrefix = clashFingerprintPrefix
				}
				for proxySwitches < maxProxySwitches {
					proxySwitches++
					clashMu.Lock()
					selection, err := clashClient.SwitchToNextAvailable(taskCtx)
					clashMu.Unlock()
					if err != nil {
						log.Printf("[Kiro][%d/%d] models 403 后切换 Clash 节点重验活失败 (%d/%d): %v",
							i+1, displayTotal, proxySwitches, maxProxySwitches, err)
						break
					}
					locale := applyClashSelectionToConfigForSubject(&attemptCfg, attemptCfg.Proxy, selection, currentClashFingerprintPrefix, fingerprintSubjectForTask(&attemptCfg, currentEmail))
					currentClashNode = selection.Node
					delayText := "跳过连通性测试"
					if !selection.SkippedTest {
						delayText = fmt.Sprintf("延迟 %dms", selection.DelayMs)
					}
					log.Printf("[Kiro][%d/%d] models 403 后切换 Clash 节点重验活: %s / %s (%s, 尝试 %d 个, 耗时 %dms)",
						i+1, displayTotal, selection.ProxyGroup, selection.Node, delayText, selection.Attempts, selection.DurationMs)
					log.Printf("[Kiro][%d/%d] 重验活浏览器地区已绑定: acceptLanguage=%s, i18next=%s, timeZone=%d",
						i+1, displayTotal, locale.AcceptLanguage, locale.I18Next, locale.TimeZone)

					rebuilt, alive := reverifyRegistrationResult(&attemptCfg, result)
					result = rebuilt
					if resultEmail, _ := result["email"].(string); resultEmail != "" {
						currentEmail = resultEmail
					}
					if alive {
						clashMu.Lock()
						clashClient.RecordNodeSuccess(currentClashNode)
						clashMu.Unlock()
						log.Printf("[Kiro][%d/%d] 切换 Clash 节点后验活成功，保留已注册账号: %s", i+1, displayTotal, currentEmail)
						break
					}

					retryErr, _ := result["error"].(string)
					switch {
					case isProxyNetworkError(retryErr):
						statsMu.Lock()
						runtimeStats.clashNetworkErrors++
						statsMu.Unlock()
						clashMu.Lock()
						clashClient.RecordNodeNetworkFailure(currentClashNode)
						clashMu.Unlock()
					case isClashNodeRiskFailure(retryErr):
						statsMu.Lock()
						runtimeStats.clashRiskFailures++
						statsMu.Unlock()
						clashMu.Lock()
						clashClient.RecordNodeRiskFailure(currentClashNode)
						clashMu.Unlock()
						log.Printf("[Kiro][%d/%d] 重验活 Clash 节点仍触发风控，临时隔离: %s (%s)", i+1, displayTotal, currentClashNode, retryErr)
					default:
						log.Printf("[Kiro][%d/%d] 切换 Clash 节点后验活仍失败: %s", i+1, displayTotal, retryErr)
						clashMu.Lock()
						clashClient.RecordNodeSuccess(currentClashNode)
						clashMu.Unlock()
					}
					if !isModelVerificationAccessDeniedError(retryErr) && !isProxyNetworkError(retryErr) {
						break
					}
				}
				if result["status"] == "success" {
					break
				}
				errorMsg, _ = result["error"].(string)
				proxyNetworkErr = isProxyNetworkError(errorMsg)
			}

			// AWS/TES 熔断：普通数量模式下任一任务遇到明确 BLOCKED/TES 类错误即终止全部。
			// 成功目标模式下失败尝试应继续补齐成功数；这类错误只失败当前尝试，不提前结束批次。
			if shouldForceStopTaskForMode(errorMsg, emailProvider, killSwitchEnabled, successTargetMode) {
				otpKillOnce.Do(func() {
					log.Printf("[Kiro] ⚠️ 检测到不可继续错误(%s)，立即终止所有注册任务", errorMsg)
					go StopTask(true)
				})
				break
			}

			// 邮箱已注册：标记当前账号，换号重来（重置 attempt）
			if taskCfg.UseOutlook && strings.Contains(errorMsg, "邮箱已注册过") {
				log.Printf("[Kiro][%d/%d] %s 已注册，标记并换号", i+1, displayTotal, currentEmail)
				statsMu.Lock()
				runtimeStats.RecordRegisteredSkip()
				statsMu.Unlock()
				publishDiagnostics()
				email.UpdateAccountStatus(accountEmail, true, false, "邮箱已注册")
				if taskCfg.OutlookAccount != nil {
					registrationTracker.MarkRegistered(*taskCfg.OutlookAccount)
				}
				if nextUsableOutlookAccount() {
					taskCfg.Password = core.GenPassword()
					attempt, proxySwitches = resetOutlookRetryBudgetAfterAccountRotation(attempt, proxySwitches)
					continue retryLoop
				}
				// 账号池耗尽
				log.Printf("[Kiro][%d/%d] 账号池已耗尽", i+1, displayTotal)
				if successTargetMode && onlyOutlookProvider {
					requestTaskStop("[Kiro] Outlook 账号池已耗尽，停止成功目标模式")
				}
				break
			}

			// Outlook Graph refresh_token 失效/风控账号：send-otp 已发出，但该邮箱不可收信，
			// 标记异常并立即换下一个 Outlook 账号，避免卡在同一坏账号上等待/重试。
			if taskCfg.UseOutlook && shouldRotateOutlookAccountAfterFailure(errorMsg) {
				failReason := classifyError(errorMsg)
				log.Printf("[Kiro][%d/%d] Outlook 账号异常，标记并换号: %s (%s)", i+1, displayTotal, currentEmail, failReason)
				email.UpdateAccountStatus(accountEmail, true, false, failReason)
				if nextUsableOutlookAccount() {
					taskCfg.Password = core.GenPassword()
					attempt, proxySwitches = resetOutlookRetryBudgetAfterAccountRotation(attempt, proxySwitches)
					continue retryLoop
				}
				log.Printf("[Kiro][%d/%d] Outlook 账号池已耗尽", i+1, displayTotal)
				if successTargetMode && onlyOutlookProvider {
					requestTaskStop("[Kiro] Outlook 账号池已耗尽，停止成功目标模式")
				}
				break
			}

			// models 403 发生在 SetPassword 之后：邮箱已被消耗，但该账号无法正常拉模型。
			// 对 Outlook 账号池而言，这类账号应标记为已注册失败并换下一个账号补齐当前序号，
			// 避免 Count=3 时因为账号级模型权限未放行而直接变成 0/3 或 2/3。
			if shouldRotateOutlookAfterPostPasswordAccountFailure(taskCfg.UseOutlook, result) {
				failReason := classifyError(errorMsg)
				log.Printf("[Kiro][%d/%d] Outlook 账号注册后不可用，标记并换号补齐: %s (%s)", i+1, displayTotal, currentEmail, failReason)
				if currentClashAssisted && currentClashNode != "" && isPostPasswordSuspendedAccountFailure(errorMsg) {
					postPasswordSuspendedRegionsMu.Lock()
					regionKey, regionCount, cooldownRegion := recordPostPasswordSuspendedRegion(postPasswordSuspendedRegions, currentClashNode)
					postPasswordSuspendedRegionsMu.Unlock()
					if cooldownRegion {
						clashMu.Lock()
						clashClient.RecordNodeRegionRiskFailure(currentClashNode)
						clashMu.Unlock()
						log.Printf("[Kiro][%d/%d] 同地区连续账号暂锁，短期跳过该 Clash 地区节点簇: region=%s count=%d node=%s",
							i+1, displayTotal, regionKey, regionCount, currentClashNode)
					}
				}
				statsMu.Lock()
				runtimeStats.RecordFailure(errorMsg, true, "验活")
				statsMu.Unlock()
				publishDiagnostics()
				email.UpdateAccountStatus(accountEmail, true, false, failReason)
				if nextUsableOutlookAccount() {
					taskCfg.Password = core.GenPassword()
					attempt, proxySwitches = resetOutlookRetryBudgetAfterAccountRotation(attempt, proxySwitches)
					continue retryLoop
				}
				log.Printf("[Kiro][%d/%d] Outlook 账号池已耗尽", i+1, displayTotal)
				if successTargetMode && onlyOutlookProvider {
					requestTaskStop("[Kiro] Outlook 账号池已耗尽，停止成功目标模式")
				}
				break
			}

			// Point of no return：Step12 已完成但整体失败 → 邮箱已消耗，不换代理重试
			if pwSet, _ := result["passwordSet"].(bool); pwSet {
				log.Printf("[Kiro][%d/%d] 密码已设置但验活失败，邮箱已消耗，不再重试", i+1, displayTotal)
				break
			}

			// 动态代理池中部分 UUID 节点可能不可用；代理类网络错误优先切换新 UUID，不消耗业务重试次数。
			if (proxy.HasURLTemplate(taskCfg.Proxy) || currentClashAssisted) && proxyNetworkErr && proxySwitches < maxProxySwitches {
				proxySwitches++
				if currentClashAssisted {
					log.Printf("[Kiro][%d/%d] 检测到 Clash 节点网络错误，切换下一个节点重试 (%d/%d): %s",
						i+1, displayTotal, proxySwitches, maxProxySwitches, errorMsg)
				} else {
					log.Printf("[Kiro][%d/%d] 检测到代理节点网络错误，切换新 UUID 节点重试 (%d/%d): %s",
						i+1, displayTotal, proxySwitches, maxProxySwitches, errorMsg)
				}
				attempt--
				continue retryLoop
			}

			// Step6 提交邮箱 403 等发生在验证码/创建身份之前：邮箱和账号尚未消耗。
			// 优先同邮箱切换动态代理重试；超过切换上限后作为前置补齐失败释放注册名额。
			if shouldTreatReservedFailureAsPreflight(result) && (proxy.HasURLTemplate(taskCfg.Proxy) || currentClashAssisted) && proxySwitches < maxProxySwitches {
				proxySwitches++
				if currentClashAssisted {
					log.Printf("[Kiro][%d/%d] 预注册阶段触发风控，切换下一个 Clash 节点重试 (%d/%d): %s",
						i+1, displayTotal, proxySwitches, maxProxySwitches, errorMsg)
				} else {
					log.Printf("[Kiro][%d/%d] 预注册阶段触发风控，切换新 UUID 节点重试 (%d/%d): %s",
						i+1, displayTotal, proxySwitches, maxProxySwitches, errorMsg)
				}
				attempt--
				continue retryLoop
			}

			// 不重试的错误类型（含 context 取消 / 被封 / 临时邮箱重复）。
			// 临时邮箱 send-otp/TES BLOCKED 只失败当前邮箱，不在同一邮箱上重复打。
			noRetryErrors := []string{"suspended", "临时邮箱不可能已存在", "邮箱创建失败", "context canceled", "context deadline exceeded"}
			shouldRetry := shouldRetrySameMailboxAfterFailure(errorMsg, emailProvider)
			for _, noRetry := range noRetryErrors {
				if strings.Contains(errorMsg, noRetry) {
					shouldRetry = false
					break
				}
			}

			if !shouldRetry || attempt >= maxAttempts-1 {
				break
			}

			log.Printf("[Kiro][%d/%d] 注册失败: %s，准备重试", i+1, displayTotal, errorMsg)
		}

		if registrationReserved && shouldTreatReservedFailureAsPreflight(result) {
			errorMsg, _ := result["error"].(string)
			releaseRegistrationReservation()
			Manager.mu.Lock()
			completedCount := Manager.completed
			successCount := Manager.success
			failedCount := Manager.failed
			Manager.mu.Unlock()
			log.Printf("[Kiro][%d/%d] 预注册失败（未进入验证码/创建身份流程，不计入注册次数，继续补齐）: %s", i+1, displayTotal, errorMsg)
			statsMu.Lock()
			runtimeStats.RecordPreflightRegistrationFailure(errorMsg)
			progressSummary := runtimeStats.ProgressSummary(completedCount, successCount, failedCount, 3)
			statsMu.Unlock()
			publishDiagnostics()
			log.Println(progressSummary)
			return
		}

		itemDuration := time.Since(itemStart).Seconds()

		Manager.mu.Lock()
		Manager.results = append(Manager.results, result)
		Manager.completed++

		success := result["status"] == "success"
		if success {
			Manager.success++
		} else {
			Manager.failed++
		}
		completedCount := Manager.completed
		successCount := Manager.success
		failedCount := Manager.failed
		Manager.mu.Unlock()

		// 统计分类：失败时计算分类原因，供统计打印与邮箱状态标记复用
		var failReason string
		statsMu.Lock()
		taskDurations = append(taskDurations, itemDuration)
		if !success {
			errorMsg, _ := result["error"].(string)
			passwordSet, _ := result["passwordSet"].(bool)
			failReason = runtimeStats.RecordFailure(errorMsg, passwordSet, "")
			if diag, ok := parseSendOTPDiagnostics(errorMsg); ok {
				sendOTPDiagnostics = append(sendOTPDiagnostics, diag)
			}
		}
		shouldStopForOTPTimeouts := shouldStopForOutlookOTPTimeout(taskCfg.UseOutlook, success, failReason, successTargetMode, &otpTimeoutStreak)
		if success || (!success && failReason != "验证码超时") {
			otpTimeoutStreak.Record(failReason)
		}
		if shouldStopForOTPTimeouts {
			otpTimeoutStopTriggered = true
		}
		statsMu.Unlock()
		publishDiagnostics()

		if shouldStopForOTPTimeouts {
			requestTaskStop("[Kiro] 连续验证码超时达到 5 次，停止本批任务；请检查 Outlook Graph 收信、别名投递或邮箱代理")
		}
		recordEmailProviderSuccessStat(emailProvider, result)

		recycleErrorMsg, _ := result["error"].(string)
		if candidate, ok := recycleReusableFailedEmail(req, &reusableEmails, emailProvider, &taskCfg, result, shouldForceStopTaskForMode(recycleErrorMsg, emailProvider, killSwitchEnabled, successTargetMode)); ok {
			log.Printf("[Kiro][%d/%d] 回收可复用临时邮箱: %s", completedCount, displayTotal, candidate.address)
		}

		// log.Printf 必须在 state.mu 外调用，否则与 logWriter 死锁
		if !success {
			if errMsg, ok := result["error"].(string); ok {
				log.Printf("[Kiro][%d/%d] 失败: %s (%s)", completedCount, displayTotal, errMsg, currentEmail)
			}
		}
		statsMu.Lock()
		progressSummary := runtimeStats.ProgressSummary(completedCount, successCount, failedCount, 3)
		statsMu.Unlock()
		publishDiagnostics()
		log.Println(progressSummary)

		// 邮箱状态标记：registered 仍仅在设密码后置为 true（保持可重试语义不变），
		// 但失败原因 failReason 无论失败发生在哪个阶段都记录，供前端按类型筛选。
		if taskCfg.UseOutlook && accountEmail != "" {
			passwordSet, _ := result["passwordSet"].(bool)
			if passwordSet {
				// 已走到设密码步骤：正式标记 registered，成功则清除失败原因
				email.UpdateAccountStatus(accountEmail, true, success, failReason)
			} else if !success {
				// 前置阶段失败（如验证码超时）：不标记 registered，邮箱可被下次任务重试，
				// 仅记录最近一次失败原因。
				email.MarkAccountFailReason(accountEmail, failReason)
			}
		}
		if success {
			if err := data.SaveKiroSuccess(result, outDir); err != nil {
				log.Printf("[Kiro] 保存结果失败: %v", err)
			} else {
				enqueueKiroRSAutoSyncResult(outDir, result)
			}
		}
		if successTargetMode && successCount >= successTarget {
			requestTaskStop(fmt.Sprintf("[Kiro] 注册成功数量已达到 %d，停止继续注册", successTarget))
		}
	}

	if successTargetMode {
		workerCount := req.Concurrency
		if workerCount < 1 {
			workerCount = 1
		}
		if workerCount > successTarget {
			workerCount = successTarget
		}
		log.Printf("[Kiro] 启动成功目标任务: 目标成功 %d 个，并发数 %d", successTarget, workerCount)
		log.Printf("[Kiro] 并发任务启动错峰: 步进 100ms, 抖动 0-80ms")
		var wg sync.WaitGroup
		for workerID := 0; workerID < workerCount; workerID++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					idx, ok := nextSuccessTargetAttempt()
					if !ok {
						return
					}
					if idx < workerCount {
						stagger := concurrentStartStagger(idx, workerCount)
						if stagger > 0 {
							timer := time.NewTimer(stagger)
							select {
							case <-Manager.stopCh:
								if !timer.Stop() {
									<-timer.C
								}
								finishSuccessTargetAttempt()
								return
							case <-timer.C:
							}
						}
					}

					doTask(idx)
					finishSuccessTargetAttempt()

					// 串行成功目标模式保留任务间隔语义，并允许停止信号中断等待。
					if workerCount == 1 && req.Delay > 0 {
						timer := time.NewTimer(time.Duration(req.Delay) * time.Second)
						select {
						case <-Manager.stopCh:
							if !timer.Stop() {
								<-timer.C
							}
							return
						case <-timer.C:
						}
					}
				}
			}()
		}
		wg.Wait()
	} else if req.Concurrency > 1 {
		log.Printf("[Kiro] 启动并发任务: %d 个任务，并发数 %d", req.Count, req.Concurrency)
		log.Printf("[Kiro] 并发任务启动错峰: 步进 100ms, 抖动 0-80ms")
		workerCount := req.Concurrency
		var wg sync.WaitGroup
		for workerID := 0; workerID < workerCount; workerID++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					idx, ok := nextPreparationAttempt()
					if !ok {
						return
					}

					stagger := concurrentStartStagger(idx, workerCount)
					if stagger > 0 {
						timer := time.NewTimer(stagger)
						select {
						case <-Manager.stopCh:
							if !timer.Stop() {
								<-timer.C
							}
							return
						case <-timer.C:
						}
					}

					doTask(idx)
				}
			}()
		}
		wg.Wait()
	} else {
		log.Printf("[Kiro] 启动串行任务: %d 个任务", req.Count)
	serialLoop:
		for {
			idx, ok := nextPreparationAttempt()
			if !ok {
				break serialLoop
			}
			select {
			case <-Manager.stopCh:
				log.Println("任务已停止")
				// 跳出循环而非直接 return，确保走到末尾的统计与自动同步逻辑，
				// 与并发模式行为一致：停止前已成功的账号仍会被同步。
				break serialLoop
			default:
			}
			doTask(idx)
			if req.Delay > 0 && !registrationCounter.done() {
				time.Sleep(time.Duration(req.Delay) * time.Second)
			}
		}
	}

	totalDuration := time.Since(taskStartTime).Seconds()

	Manager.mu.Lock()
	sucCount := Manager.success
	failCount := Manager.failed
	totalCount := Manager.completed
	Manager.mu.Unlock()

	// 计算平均耗时
	var avgDur float64
	if len(taskDurations) > 0 {
		var sum float64
		for _, d := range taskDurations {
			sum += d
		}
		avgDur = sum / float64(len(taskDurations))
	}

	// 统计报告
	log.Println("[Kiro] ═══════════════════════════════")
	log.Printf("[Kiro] 任务完成 — 总计: %d, 成功: %d, 失败: %d", totalCount, sucCount, failCount)
	log.Printf("[Kiro] 总耗时: %.1fs, 平均耗时: %.1fs/个", totalDuration, avgDur)
	if totalCount > 0 {
		log.Printf("[Kiro] 成功率: %.1f%%", float64(sucCount)/float64(totalCount)*100)
	}
	if failCount > 0 {
		log.Printf("[Kiro] 失败明细:")
		type failEntry struct {
			name  string
			count int
		}
		var entries []failEntry
		for name, count := range runtimeStats.failCategories {
			entries = append(entries, failEntry{name, count})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].count > entries[j].count
		})
		for _, e := range entries {
			log.Printf("[Kiro]   %s: %d (%.0f%%)", e.name, e.count, float64(e.count)/float64(totalCount)*100)
		}
		statsMu.Lock()
		regSkip := runtimeStats.registeredSkipCount
		graphFails := runtimeStats.graphResolveFailures
		clashNetErrs := runtimeStats.clashNetworkErrors
		clashRiskErrs := runtimeStats.clashRiskFailures
		otpStopped := otpTimeoutStopTriggered
		networkStages := make(map[string]int, len(runtimeStats.networkStageFailures))
		for k, v := range runtimeStats.networkStageFailures {
			networkStages[k] = v
		}
		clashQuarantined := 0
		if clashClient != nil {
			clashQuarantined = clashClient.QuarantinedNodeCount()
		}
		poolQuarantined := proxy.QuarantinedPoolProxyCount()
		statsMu.Unlock()
		if regSkip > 0 || graphFails > 0 || clashNetErrs > 0 || clashRiskErrs > 0 || clashQuarantined > 0 || poolQuarantined > 0 || otpStopped {
			log.Printf("[Kiro] 优化诊断: 已注册跳过=%d, Graph地址解析失败=%d, Clash网络错误=%d, Clash风控节点=%d, Clash临时拉黑节点=%d, 代理池临时拉黑=%d, 连续验证码超时熔断=%v",
				regSkip, graphFails, clashNetErrs, clashRiskErrs, clashQuarantined, poolQuarantined, otpStopped)
		}
		if len(networkStages) > 0 {
			type stageEntry struct {
				name  string
				count int
			}
			stages := make([]stageEntry, 0, len(networkStages))
			for name, count := range networkStages {
				stages = append(stages, stageEntry{name: name, count: count})
			}
			sort.Slice(stages, func(i, j int) bool {
				if stages[i].count == stages[j].count {
					return stages[i].name < stages[j].name
				}
				return stages[i].count > stages[j].count
			})
			parts := make([]string, 0, len(stages))
			for _, stage := range stages {
				parts = append(parts, fmt.Sprintf("%s:%d", stage.name, stage.count))
			}
			log.Printf("[Kiro] 网络/代理错误阶段分布: %s", strings.Join(parts, "；"))
		}
		if top := runtimeStats.topFailures(10); top != "-" {
			log.Printf("[Kiro] 失败Top: %s", top)
		}
		if summary := failureDiagnosisSummary(runtimeStats.failCategories, totalCount, sucCount); summary != "" {
			log.Printf("[Kiro] 诊断建议: %s", summary)
		}
		if summary := sendOTPDiagnosticsSummary(sendOTPDiagnostics); summary != "" {
			log.Printf("[Kiro] send-otp 诊断汇总: %s", summary)
		}
	}
	runtimeStats.logPreflightFailureSummary()
	if sucCount > 0 {
		log.Printf("[Kiro] 成功结果: %s", outDir)
	}
	log.Println("[Kiro] ═══════════════════════════════")

}

func selectAccountsByEmail(accounts []map[string]interface{}, emails []string) ([]map[string]interface{}, []string) {
	if len(accounts) == 0 || len(emails) == 0 {
		missing := make([]string, 0, len(emails))
		for _, email := range emails {
			if strings.TrimSpace(email) != "" {
				missing = append(missing, email)
			}
		}
		return nil, missing
	}
	wanted := make(map[string]struct{}, len(emails))
	original := make(map[string]string, len(emails))
	for _, email := range emails {
		if key := strings.ToLower(strings.TrimSpace(email)); key != "" {
			wanted[key] = struct{}{}
			if _, exists := original[key]; !exists {
				original[key] = email
			}
		}
	}
	out := make([]map[string]interface{}, 0, len(wanted))
	found := make(map[string]struct{}, len(wanted))
	for _, account := range accounts {
		email, _ := account["email"].(string)
		key := strings.ToLower(strings.TrimSpace(email))
		if _, ok := wanted[key]; ok {
			out = append(out, account)
			found[key] = struct{}{}
		}
	}
	missing := make([]string, 0)
	for _, email := range emails {
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" {
			continue
		}
		if _, ok := found[key]; !ok {
			missing = append(missing, original[key])
		}
	}
	return out, missing
}

func successfulSyncEmails(result kirorsync.SyncResult) []string {
	emails := make([]string, 0, result.Success)
	for _, detail := range result.Details {
		if detail.Success && strings.TrimSpace(detail.Email) != "" {
			emails = append(emails, detail.Email)
		}
	}
	return emails
}

func rejectedSyncEmails(result kirorsync.SyncResult) []string {
	emails := make([]string, 0)
	for _, detail := range result.Details {
		if detail.Rejected && strings.TrimSpace(detail.Email) != "" {
			emails = append(emails, detail.Email)
		}
	}
	return emails
}

func applyKiroRSSyncResult(outDir string, result kirorsync.SyncResult) (int, int, []string, error) {
	updated, err := data.MarkKiroRSSynced(outDir, successfulSyncEmails(result))
	if err != nil {
		return updated, 0, nil, fmt.Errorf("更新同步状态失败: %w", err)
	}

	rejectedEmails := rejectedSyncEmails(result)
	removed := 0
	if len(rejectedEmails) > 0 {
		removed, err = data.DeleteAccounts(outDir, rejectedEmails)
		if err != nil {
			return updated, removed, rejectedEmails, fmt.Errorf("删除本地失效账号失败: %w", err)
		}
	}
	return updated, removed, rejectedEmails, nil
}

// classifyError 根据错误信息粗分类，用于统计展示。
func classifyError(errorMsg string) string {
	if errorMsg == "" {
		return "未知错误"
	}
	lower := strings.ToLower(errorMsg)
	if strings.Contains(errorMsg, "任务已取消") || strings.Contains(lower, "context canceled") {
		return "任务取消"
	}
	if isOutlookGraphInvalidGrantError(errorMsg) {
		return "Graph Token失效"
	}
	if strings.Contains(errorMsg, "Outlook Graph 地址解析失败") || strings.Contains(errorMsg, "Graph /me") || strings.Contains(errorMsg, "Graph 查询失败") {
		return classifyGraphResolutionError(errorMsg)
	}
	if strings.Contains(errorMsg, "验活失败") {
		return "验活失败"
	}
	if strings.Contains(errorMsg, "KiroAuthorize") || strings.Contains(errorMsg, "授权Kiro") || strings.Contains(errorMsg, "Kiro访问") {
		return "Kiro授权失败"
	}
	if strings.Contains(errorMsg, "KiroExchange") || strings.Contains(errorMsg, "访问令牌") || strings.Contains(errorMsg, "交换令牌") {
		return "Token交换失败"
	}
	if strings.Contains(errorMsg, "SetPassword") || strings.Contains(errorMsg, "设置密码") {
		return "密码设置失败"
	}
	if strings.Contains(errorMsg, "suspended") || strings.Contains(errorMsg, "封禁") {
		return "账号封禁"
	}
	if strings.Contains(errorMsg, "邮箱已注册") || strings.Contains(errorMsg, "临时邮箱不可能已存在") {
		return "邮箱已注册"
	}
	if strings.Contains(errorMsg, "验证码接收超时") || strings.Contains(errorMsg, "等待验证码超时") {
		return "验证码超时"
	}
	if strings.Contains(errorMsg, "INVALID_OTP") || strings.Contains(errorMsg, "验证码错误") || strings.Contains(errorMsg, "验证码无效") {
		return "验证码无效"
	}
	if isSubmitEmail403RiskError(errorMsg) {
		return "IP/指纹风控"
	}
	if isSendOTPBlockedError(errorMsg) {
		return "IP/指纹风控"
	}
	if strings.Contains(errorMsg, "send-otp 失败") {
		return "验证码发送失败"
	}
	if strings.Contains(errorMsg, "IP或浏览器指纹") || strings.Contains(errorMsg, "注册被拦截") || strings.Contains(errorMsg, "BLOCKED") {
		return "IP/指纹风控"
	}
	if strings.Contains(errorMsg, "请求过于频繁") {
		return "请求频率限制"
	}
	if strings.Contains(errorMsg, "邮箱服务异常") || strings.Contains(errorMsg, "获取邮件失败") || strings.Contains(errorMsg, "邮箱创建失败") {
		return "邮箱服务异常"
	}
	if strings.Contains(errorMsg, "服务暂时不可用") {
		return "服务不可用"
	}
	if strings.Contains(errorMsg, "加密失败") || strings.Contains(errorMsg, "JWE") {
		return "加密服务异常"
	}
	if isProxyNetworkError(errorMsg) || strings.Contains(lower, "timeout") || strings.Contains(errorMsg, "网络") || strings.Contains(lower, "connection") || strings.Contains(lower, "tls") || strings.Contains(errorMsg, "代理") {
		return "网络/代理问题"
	}
	return "其他错误"
}

func classifyGraphResolutionError(errorMsg string) string {
	lower := strings.ToLower(strings.TrimSpace(errorMsg))
	switch {
	case strings.Contains(lower, "invalid_grant") || strings.Contains(lower, "aadsts") || strings.Contains(lower, "service abuse"):
		return "Graph Token失效"
	case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		return "Graph权限错误"
	case strings.Contains(lower, "403") || strings.Contains(lower, "forbidden"):
		return "Graph权限错误"
	case isProxyNetworkError(errorMsg):
		return "Graph网络错误"
	default:
		return "Graph响应异常"
	}
}

func classifyNetworkErrorStage(errorMsg string) string {
	switch {
	case strings.Contains(errorMsg, "设备注册失败"):
		return "设备授权"
	case strings.Contains(errorMsg, "门户访问失败"):
		return "Portal 初始化"
	case strings.Contains(errorMsg, "工作流初始化失败"):
		return "工作流初始化"
	case strings.Contains(errorMsg, "提交邮箱失败"):
		return "提交邮箱"
	case strings.Contains(errorMsg, "send-otp") || strings.Contains(errorMsg, "发送验证码"):
		return "发送验证码"
	case strings.Contains(errorMsg, "等待验证码") || strings.Contains(errorMsg, "获取邮件失败"):
		return "等待验证码"
	case strings.Contains(errorMsg, "配置初始化失败") || strings.Contains(errorMsg, "配置启动失败") || strings.Contains(errorMsg, "初始化注册失败"):
		return "注册初始化"
	case strings.Contains(errorMsg, "验活失败"):
		return "验活"
	default:
		return "其他阶段"
	}
}

func isSubmitEmail403RiskError(errorMsg string) bool {
	lower := strings.ToLower(errorMsg)
	return strings.Contains(errorMsg, "提交邮箱失败") &&
		(strings.Contains(lower, "403") || strings.Contains(lower, "forbidden"))
}

func shouldTreatReservedFailureAsPreflight(result map[string]interface{}) bool {
	if result == nil || result["status"] == "success" {
		return false
	}
	if passwordSet, _ := result["passwordSet"].(bool); passwordSet {
		return false
	}
	errorMsg, _ := result["error"].(string)
	errorMsg = strings.TrimSpace(errorMsg)
	if errorMsg == "" {
		return false
	}
	if isSubmitEmail403RiskError(errorMsg) {
		return true
	}
	lower := strings.ToLower(errorMsg)
	if strings.Contains(errorMsg, "代理无可用节点") || strings.Contains(errorMsg, "代理池无可用节点") {
		return true
	}
	if strings.Contains(lower, "context canceled") || strings.Contains(errorMsg, "任务已取消") {
		return true
	}
	return false
}

func riskFailureDetail(errorMsg, reason string) string {
	switch {
	case isSendOTPBlockedError(errorMsg):
		return "send-otp TES/BLOCKED"
	case isSubmitEmail403RiskError(errorMsg):
		return "提交邮箱 403"
	case strings.Contains(errorMsg, "注册被拦截"):
		return "注册被拦截"
	case strings.TrimSpace(reason) == "IP/指纹风控":
		return "IP/指纹风控"
	default:
		return ""
	}
}

func postRegistrationFailureDetail(errorMsg, fallbackReason string) string {
	if isModelVerificationAccessDeniedError(errorMsg) {
		return "模型列表失败"
	}
	reason := strings.TrimSpace(fallbackReason)
	if reason == "" {
		return "其他错误"
	}
	return reason
}

func emailServiceFailureDetail(errorMsg, reason string) string {
	switch {
	case strings.Contains(errorMsg, "邮箱创建失败") || strings.Contains(errorMsg, "生成") && strings.Contains(errorMsg, "邮箱失败"):
		return "邮箱创建失败"
	case strings.Contains(errorMsg, "获取邮件失败") || strings.Contains(errorMsg, "获取邮件列表失败") || strings.Contains(errorMsg, "获取邮件详情失败"):
		return "获取邮件失败"
	case strings.TrimSpace(reason) == "邮箱已注册":
		return "邮箱已注册"
	case strings.TrimSpace(reason) == "邮箱服务异常":
		return "邮箱服务异常"
	default:
		return ""
	}
}

func resetOutlookRetryBudgetAfterAccountRotation(attempt, proxySwitches int) (int, int) {
	return -1, 0
}
func isOutlookGraphInvalidGrantError(errorMsg string) bool {
	lower := strings.ToLower(strings.TrimSpace(errorMsg))
	if lower == "" {
		return false
	}
	if !strings.Contains(lower, "刷新 outlook graph token 失败") {
		return false
	}
	return strings.Contains(lower, "invalid_grant") ||
		strings.Contains(lower, "service abuse") ||
		strings.Contains(lower, "aadsts70000")
}

func shouldRotateOutlookAccountAfterFailure(errorMsg string) bool {
	return isOutlookGraphInvalidGrantError(errorMsg)
}

func failureDiagnosisSummary(failCategories map[string]int, totalCount int, successCount int) string {
	if successCount > 0 || totalCount <= 0 || len(failCategories) == 0 {
		return ""
	}
	windControl := failCategories["IP/指纹风控"]
	sendOTPFailed := failCategories["验证码发送失败"]
	if windControl*2 >= totalCount {
		return "send-otp 阶段以 TES/BLOCKED 为主，当前配置更像代理/IP/指纹被风控；建议先停止批量重试并检查上方 provider/domain/proxy 诊断"
	}
	if sendOTPFailed*2 >= totalCount {
		return "send-otp 普通 400 占比过高，更像邮箱域名或邮件提供商被拒；建议根据上方 provider/domain 诊断更换合规邮件提供商或稍后重试"
	}
	return ""
}

func parseSendOTPDiagnostics(errorMsg string) (map[string]string, bool) {
	start := strings.LastIndex(errorMsg, "[")
	end := strings.LastIndex(errorMsg, "]")
	if start < 0 || end <= start {
		return nil, false
	}
	raw := strings.TrimSpace(errorMsg[start+1 : end])
	if raw == "" || (!strings.Contains(raw, "provider=") && !strings.Contains(raw, "domain=")) {
		return nil, false
	}
	out := make(map[string]string)
	for _, part := range strings.Split(raw, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		switch key {
		case "provider", "domain", "emailProxy", "proxy":
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func sendOTPDiagnosticsSummary(items []map[string]string) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	for _, key := range []string{"provider", "domain", "emailProxy", "proxy"} {
		if top := topDiagnosticValue(items, key); top != "" {
			parts = append(parts, top)
		}
	}
	return strings.Join(parts, "；")
}

func topDiagnosticValue(items []map[string]string, key string) string {
	counts := make(map[string]int)
	for _, item := range items {
		value := strings.TrimSpace(item[key])
		if value == "" {
			continue
		}
		counts[value]++
	}
	if len(counts) == 0 {
		return ""
	}
	type entry struct {
		value string
		count int
	}
	entries := make([]entry, 0, len(counts))
	for value, count := range counts {
		entries = append(entries, entry{value: value, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count == entries[j].count {
			return entries[i].value < entries[j].value
		}
		return entries[i].count > entries[j].count
	})
	return fmt.Sprintf("%s=%s:%d", key, entries[0].value, entries[0].count)
}

// isProxyNetworkError 判断错误是否更像代理节点不可用，而不是业务风控或邮箱问题。
func isProxyNetworkError(errorMsg string) bool {
	if errorMsg == "" {
		return false
	}
	if isSendOTPBlockedError(errorMsg) || strings.Contains(errorMsg, "注册被拦截") {
		return false
	}
	lower := strings.ToLower(errorMsg)
	if lower == "eof" || strings.Contains(lower, ": eof") || strings.Contains(lower, " eof") {
		return true
	}
	triggers := []string{
		"timeout",
		"i/o timeout",
		"deadline",
		"tls handshake",
		"connection reset",
		"connection refused",
		"unexpected eof",
		"client.timeout exceeded",
		"broken pipe",
		"connect",
		"proxy",
		"代理",
		"网络连接",
		"连接超时",
		"连接失败",
		"连接被拒绝",
		"域名解析失败",
	}
	for _, trigger := range triggers {
		if strings.Contains(lower, strings.ToLower(trigger)) {
			return true
		}
	}
	return false
}

func isClashNodeRiskFailure(errorMsg string) bool {
	if errorMsg == "" {
		return false
	}
	if isModelVerificationAccessDeniedError(errorMsg) {
		return true
	}
	if isSendOTPBlockedError(errorMsg) || strings.Contains(errorMsg, "注册被拦截") {
		return true
	}
	lower := strings.ToLower(errorMsg)
	return (strings.Contains(errorMsg, "IP或浏览器指纹") || strings.Contains(errorMsg, "IP/指纹风控")) &&
		(strings.Contains(lower, "blocked") || strings.Contains(errorMsg, "风控"))
}

func isRuntimeProxyCountryRiskFailure(errorMsg string) bool {
	if errorMsg == "" {
		return false
	}
	return isClashNodeRiskFailure(errorMsg) ||
		isSubmitEmail403RiskError(errorMsg) ||
		isPostPasswordSuspendedAccountFailure(errorMsg)
}

func isModelVerificationAccessDeniedError(errorMsg string) bool {
	lower := strings.ToLower(strings.TrimSpace(errorMsg))
	if lower == "" {
		return false
	}
	if !strings.Contains(lower, "models query failed") && !strings.Contains(lower, "端点查询失败 [models]") {
		return false
	}
	return strings.Contains(lower, "403") || strings.Contains(lower, "forbidden")
}

func shouldRotateOutlookAfterPostPasswordModelFailure(useOutlook bool, result map[string]interface{}) bool {
	if !useOutlook || result == nil || result["status"] == "success" {
		return false
	}
	passwordSet, _ := result["passwordSet"].(bool)
	if !passwordSet {
		return false
	}
	errorMsg, _ := result["error"].(string)
	return isModelVerificationAccessDeniedError(errorMsg)
}

func shouldRotateOutlookAfterPostPasswordAccountFailure(useOutlook bool, result map[string]interface{}) bool {
	if !useOutlook || result == nil || result["status"] == "success" {
		return false
	}
	passwordSet, _ := result["passwordSet"].(bool)
	if !passwordSet {
		return false
	}
	errorMsg, _ := result["error"].(string)
	return isModelVerificationAccessDeniedError(errorMsg) || strings.Contains(strings.ToLower(errorMsg), "suspended") || strings.Contains(errorMsg, "封禁")
}

func isPostPasswordSuspendedAccountFailure(errorMsg string) bool {
	lower := strings.ToLower(strings.TrimSpace(errorMsg))
	return strings.Contains(lower, "suspended") || strings.Contains(errorMsg, "封禁")
}

func recordPostPasswordSuspendedRegion(counts map[string]int, node string) (string, int, bool) {
	if counts == nil {
		return "", 0, false
	}
	key := postPasswordSuspendedRegionKey(node)
	if key == "" {
		return "", 0, false
	}
	counts[key]++
	count := counts[key]
	return key, count, count >= postPasswordRegionCooldownAfter
}

func resetPostPasswordSuspendedRegion(counts map[string]int, node string) {
	if counts == nil {
		return
	}
	if key := postPasswordSuspendedRegionKey(node); key != "" {
		delete(counts, key)
	}
}

func postPasswordSuspendedRegionKey(node string) string {
	loc := core.BrowserLocaleForClashNode(node)
	key := strings.TrimSpace(loc.I18Next)
	if key == "" || key == core.DefaultBrowserLocale().I18Next {
		return ""
	}
	return key
}

func reverifyRegistrationResult(cfg *core.Config, result map[string]interface{}) (map[string]interface{}, bool) {
	if result == nil {
		return result, false
	}
	awsToken, ok := result["aws_token"].(map[string]interface{})
	if !ok || awsToken == nil {
		return result, false
	}
	reg := core.NewRegistrar(cfg)
	reg.Email, _ = result["email"].(string)
	reg.ClientID, _ = result["client_id"].(string)
	reg.ClientSecret, _ = result["client_secret"].(string)
	reg.DeviceCode, _ = result["device_code"].(string)
	verify := reg.VerifyAlive(awsToken)
	return rebuildRegistrationResultAfterReverify(result, verify)
}

func rebuildRegistrationResultAfterReverify(result, verify map[string]interface{}) (map[string]interface{}, bool) {
	if result == nil || verify == nil {
		return result, false
	}
	rebuilt := make(map[string]interface{}, len(result)+1)
	for k, v := range result {
		rebuilt[k] = v
	}
	rebuilt["verify"] = verify
	rebuilt["passwordSet"] = true
	if suspended, _ := verify["suspended"].(bool); suspended {
		rebuilt["status"] = "failed"
		rebuilt["error"] = "suspended"
		return rebuilt, false
	}
	if alive, _ := verify["alive"].(bool); !alive {
		errMsg, _ := verify["error"].(string)
		if strings.TrimSpace(errMsg) == "" {
			errMsg = "unknown"
		}
		rebuilt["status"] = "failed"
		rebuilt["error"] = "验活失败: " + errMsg
		return rebuilt, false
	}
	rebuilt["status"] = "success"
	delete(rebuilt, "error")
	return rebuilt, true
}

func isEmailProviderRateLimitError(errorMsg string) bool {
	lower := strings.ToLower(strings.TrimSpace(errorMsg))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "http 429") || strings.Contains(lower, "rate limited") || strings.Contains(lower, "too many requests")
}

func isEmailProviderAccessBlockedError(errorMsg string) bool {
	lower := strings.ToLower(strings.TrimSpace(errorMsg))
	if lower == "" {
		return false
	}
	if !strings.Contains(lower, "http 403") {
		return false
	}
	return strings.Contains(lower, "country you are requesting from") ||
		strings.Contains(lower, "not allowed to use the api free tier") ||
		strings.Contains(lower, "free tier") ||
		strings.Contains(lower, "bypass this restriction")
}

func emailProviderRateLimitBackoff(rateLimitAttempt int) time.Duration {
	if rateLimitAttempt < 0 || rateLimitAttempt >= len(emailProviderRateLimitBackoffs) {
		return 0
	}
	return emailProviderRateLimitBackoffs[rateLimitAttempt]
}

func shouldRetryEmailProviderCreateError(errorMsg string, rateLimitAttempt int) (bool, time.Duration) {
	if !isEmailProviderRateLimitError(errorMsg) {
		return false, 0
	}
	wait := emailProviderRateLimitBackoff(rateLimitAttempt)
	return wait > 0, wait
}

// 邮箱服务创建失败只代表当前邮箱渠道/出口暂时不可用，应计入本次尝试失败并继续下一次。
func shouldStopTaskForEmailCreateFailure(_ string) bool {
	return false
}

func emailProxyUsesClash(emailProxy, clashProxy string) bool {
	emailEndpoint := proxyEndpointKey(emailProxy)
	clashEndpoint := proxyEndpointKey(clashProxy)
	return emailEndpoint != "" && clashEndpoint != "" && emailEndpoint == clashEndpoint
}

func proxyEndpointKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	port := strings.TrimSpace(u.Port())
	if host == "" || port == "" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	}
	return net.JoinHostPort(host, port)
}

func createTempEmailWithRateLimitRetry(ctx context.Context, providerLabel string, taskNo, displayTotal int, create func() (string, error)) (string, error) {
	for rateLimitAttempt := 0; ; rateLimitAttempt++ {
		address, err := create()
		if err == nil {
			return address, nil
		}
		shouldRetry, wait := shouldRetryEmailProviderCreateError(err.Error(), rateLimitAttempt)
		if !shouldRetry {
			return "", err
		}
		log.Printf("[Kiro][%d/%d] %s 邮箱服务限流: %v；等待 %s 后重试 (%d/%d)", taskNo, displayTotal, providerLabel, err, wait, rateLimitAttempt+1, len(emailProviderRateLimitBackoffs))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func isSendOTPBlockedError(errorMsg string) bool {
	lower := strings.ToLower(strings.TrimSpace(errorMsg))
	if lower == "" {
		return false
	}
	return (strings.Contains(lower, "send-otp") || strings.Contains(lower, "注册被拦截")) && (strings.Contains(lower, `"errorcode":"blocked"`) ||
		strings.Contains(lower, "errorcode\":\"blocked") ||
		strings.Contains(lower, "request was blocked by tes") ||
		strings.Contains(lower, "blocked") ||
		strings.Contains(lower, "注册被拦截"))
}

func isSendOTPMailboxRejectedError(errorMsg string) bool {
	lower := strings.ToLower(strings.TrimSpace(errorMsg))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "send-otp") || strings.Contains(lower, "request was blocked by tes") || strings.Contains(lower, `"errorcode":"blocked"`) || strings.Contains(lower, "errorcode\":\"blocked") {
		return isSendOTPBlockedError(errorMsg)
	}
	if diagnostics, ok := parseSendOTPDiagnostics(errorMsg); ok {
		return isSendOTPBlockedError(errorMsg) && diagnostics["provider"] != "" && diagnostics["domain"] != ""
	}
	return false
}

// shouldRecycleReusableEmail 判断失败结果中的临时邮箱是否还可进入候选池。
func shouldRecycleReusableEmail(result map[string]interface{}) bool {
	if result == nil || result["status"] == "success" {
		return false
	}
	if passwordSet, _ := result["passwordSet"].(bool); passwordSet {
		return false
	}
	errorMsg, _ := result["error"].(string)
	return isReusableEmailError(errorMsg)
}

// isReusableEmailError 判断错误是否适合复用同一个临时邮箱换代理/指纹再试。
func isReusableEmailError(errorMsg string) bool {
	if strings.TrimSpace(errorMsg) == "" {
		return false
	}
	excludes := []string{
		"邮箱已注册",
		"邮箱已被注册",
		"临时邮箱不可能已存在",
		"INVALID_OTP",
		"验证码错误",
		"验证码无效",
		"验证码接收超时",
		"等待验证码超时",
		"邮箱创建失败",
		"获取邮件失败",
		"邮箱服务异常",
		"Mailporary 邮箱未创建",
		"suspended",
		"任务已取消",
		"context canceled",
	}
	lower := strings.ToLower(errorMsg)
	for _, exclude := range excludes {
		if strings.Contains(lower, strings.ToLower(exclude)) {
			return false
		}
	}
	triggers := []string{
		"注册被拦截",
		"IP或浏览器指纹",
		"BLOCKED",
		"注册请求被拦截",
	}
	for _, trigger := range triggers {
		if strings.Contains(lower, strings.ToLower(trigger)) {
			return true
		}
	}
	return isProxyNetworkError(errorMsg)
}

// isKillSwitchError 判断该错误是否属于"AWS 已把我们拉黑，继续跑没意义"的熔断级错误。
// Mailporary / Emailnator / mail.gw / mail.tm / TempMail.lol 的 send-otp 400 更可能是单个临时邮箱域名被拒，不能直接升级为全局熔断。
func isKillSwitchError(errorMsg, emailProvider string) bool {
	if errorMsg == "" {
		return false
	}
	if isSendOTPBlockedError(errorMsg) {
		return true
	}
	if strings.Contains(errorMsg, "send-otp 失败 (400)") {
		return !isTemporaryEmailProvider(emailProvider)
	}
	triggers := []string{
		"注册被拦截",       // formatError 对 BLOCKED/注册请求被拦截 的翻译
		"IP或浏览器指纹被检测", // 指纹/IP 被标记
		"BLOCKED",     // 响应体里直接包含的风控标记
		"注册请求被拦截",
	}
	for _, t := range triggers {
		if strings.Contains(errorMsg, t) {
			return true
		}
	}
	return false
}

func shouldRetrySameMailboxAfterFailure(errorMsg, emailProvider string) bool {
	if isTemporaryEmailProvider(emailProvider) && isSendOTPMailboxRejectedError(errorMsg) {
		return false
	}
	return true
}

func shouldForceStopTask(errorMsg, emailProvider string, killSwitchEnabled bool) bool {
	// 零配置临时邮箱的 TES/BLOCKED 常只代表当前随机邮箱域名被拒；
	// 历史成功样本显示换一个临时邮箱即可继续，因此不要全局终止批次。
	if isSendOTPBlockedError(errorMsg) && isTemporaryEmailProvider(emailProvider) {
		return false
	}
	if isSendOTPBlockedError(errorMsg) {
		return true
	}
	return killSwitchEnabled && isKillSwitchError(errorMsg, emailProvider)
}

func shouldForceStopTaskForMode(errorMsg, emailProvider string, killSwitchEnabled bool, successTargetMode bool) bool {
	if successTargetMode {
		return false
	}
	return shouldForceStopTask(errorMsg, emailProvider, killSwitchEnabled)
}

func shouldStopForOutlookOTPTimeout(useOutlook bool, success bool, failReason string, successTargetMode bool, streak *outlookOTPTimeoutStreak) bool {
	if streak == nil || !useOutlook || success {
		return false
	}
	shouldStop := streak.Record(failReason)
	return shouldStop && !successTargetMode
}

func emailProviderDisplayName(emailProvider string) string {
	if channel, ok := email.FreeCustomFixedDomainChannel(emailProvider); ok {
		return channel.Label
	}
	switch emailProvider {
	case "outlook":
		return "Outlook"
	case "moemail":
		return "MoeMail"
	case "cloudmail":
		return "cloud-mail"
	case "mailporary":
		return "Mailporary"
	case "emailnator":
		return "Emailnator"
	case "mailgw":
		return "mail.gw"
	case "mailtm":
		return "mail.tm"
	case "tempmail_lol":
		return "TempMail.lol"
	case "guerrillamail":
		return "GuerrillaMail"
	case "mailtemp":
		return "MailTemp"
	case "tempmail_plus":
		return "TempMail.plus"
	case "inboxkitten":
		return "InboxKitten"
	case "inboxes":
		return "Inboxes"
	case "freecustom":
		return "FreeCustom.Email"
	case "dropmail":
		return "DropMail"
	case "mailcatch":
		return "MailCatch"
	case "tempmailo":
		return "TempMailo"
	case "smailpro":
		return "SmailPro"
	case "tempmailbox":
		return "TempMailbox"
	case "minuteinbox":
		return "MinuteInbox"
	case "generator_email":
		return "Generator.Email"
	case "mailtowin":
		return "MailToWin"
	case "mail2me":
		return "Mail2Me"
	case "pickmemail":
		return "PickMeMail"
	case "maximail":
		return "MaxiMail"
	case "emlpro":
		return "EmlPro"
	case "freeml":
		return "FreeML"
	case "emlhub":
		return "EmlHub"
	case "emltmp":
		return "EmlTmp"
	case "mailpwr":
		return "MailPwr"
	case "tenmail":
		return "10Mail"
	case "dropmail_me":
		return "DropMail.me"
	case "mimimail":
		return "MimiMail"
	case "pickmail":
		return "PickMail"
	case "spymail":
		return "SpyMail"
	case "yomail":
		return "YoMail"
	case "tmio_bltiwd":
		return "TempMailIO bltiwd.com"
	case "tmio_wnbaldwy":
		return "TempMailIO wnbaldwy.com"
	case "tmio_bwmyga":
		return "TempMailIO bwmyga.com"
	case "tmio_ozsaip":
		return "TempMailIO ozsaip.com"
	case "gonebox":
		return "GoneBox"
	case "openinbox":
		return "OpenInbox"
	case "blinkbox":
		return "BlinkBoxApp"
	default:
		return emailProvider
	}
}

func isTemporaryEmailProvider(emailProvider string) bool {
	if _, ok := email.FreeCustomFixedDomainChannel(emailProvider); ok {
		return true
	}
	switch emailProvider {
	case "mailporary", "emailnator", "mailgw", "mailtm", "tempmail_lol", "guerrillamail", "mailtemp", "tempmail_plus", "inboxkitten", "inboxes", "freecustom", "dropmail", "mailcatch", "tempmailo", "smailpro", "tempmailbox", "minuteinbox", "generator_email", "mailtowin", "mail2me", "pickmemail", "maximail", "emlpro", "freeml", "emlhub", "emltmp", "mailpwr", "tenmail", "dropmail_me", "mimimail", "pickmail", "spymail", "yomail", "tmio_bltiwd", "tmio_wnbaldwy", "tmio_bwmyga", "tmio_ozsaip", "gonebox", "openinbox", "blinkbox":
		return true
	default:
		return false
	}
}
