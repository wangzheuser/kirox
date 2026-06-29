package task

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// State 任务状态（从原 App 脱离为独立单例）
type State struct {
	mu                   sync.Mutex
	running              bool
	stopCh               chan struct{}
	cancelFunc           context.CancelFunc // 强制取消所有 HTTP 请求
	total                int
	completed            int
	success              int
	failed               int
	successTarget        int
	successTargetEnabled bool
	results              []map[string]interface{}
	startTime            time.Time
	diagnostics          TaskDiagnostics
	logs                 []string
	logsMu               sync.Mutex
	logFile              *lumberjack.Logger // 日志文件写入器，支持自动轮转
	// OnSyncResult 同步完成回调（由 app.go 注入，用于通知前端）
	OnSyncResult func(interface{})
}

// DiagnosticGroup 表示一个可下钻诊断指标：总数 + 明细数量。
type DiagnosticGroup struct {
	Total   int            `json:"total"`
	Details map[string]int `json:"details"`
}

// DiagnosticTopItem 表示 Top 诊断项。
type DiagnosticTopItem struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// TaskDiagnostics 是实时任务状态中的结构化诊断快照。
type TaskDiagnostics struct {
	OTPFailures              DiagnosticGroup                `json:"otpFailures"`
	PostRegistrationFailures DiagnosticGroup                `json:"postRegistrationFailures"`
	NetworkProxyFailures     DiagnosticGroup                `json:"networkProxyFailures"`
	GraphFailures            DiagnosticGroup                `json:"graphFailures"`
	RiskFailures             DiagnosticGroup                `json:"riskFailures"`
	EmailServiceFailures     DiagnosticGroup                `json:"emailServiceFailures"`
	ProxyFailures            DiagnosticGroup                `json:"proxyFailures"`
	SendOTPDiagnostics       map[string][]DiagnosticTopItem `json:"sendOTPDiagnostics"`
	TopFailures              []DiagnosticTopItem            `json:"topFailures"`
}

var Manager = &State{
	logs: make([]string, 0),
}

func (g DiagnosticGroup) Clone() DiagnosticGroup {
	return DiagnosticGroup{
		Total:   g.Total,
		Details: cloneStringIntMap(g.Details),
	}
}

func (d TaskDiagnostics) Clone() TaskDiagnostics {
	sendOTP := make(map[string][]DiagnosticTopItem, len(d.SendOTPDiagnostics))
	for key, items := range d.SendOTPDiagnostics {
		copied := make([]DiagnosticTopItem, len(items))
		copy(copied, items)
		sendOTP[key] = copied
	}
	topFailures := make([]DiagnosticTopItem, len(d.TopFailures))
	copy(topFailures, d.TopFailures)
	return TaskDiagnostics{
		OTPFailures:              d.OTPFailures.Clone(),
		PostRegistrationFailures: d.PostRegistrationFailures.Clone(),
		NetworkProxyFailures:     d.NetworkProxyFailures.Clone(),
		GraphFailures:            d.GraphFailures.Clone(),
		RiskFailures:             d.RiskFailures.Clone(),
		EmailServiceFailures:     d.EmailServiceFailures.Clone(),
		ProxyFailures:            d.ProxyFailures.Clone(),
		SendOTPDiagnostics:       sendOTP,
		TopFailures:              topFailures,
	}
}

func cloneStringIntMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return map[string]int{}
	}
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func diagnosticGroup(details map[string]int) DiagnosticGroup {
	clean := make(map[string]int, len(details))
	total := 0
	for k, v := range details {
		if k == "" || v <= 0 {
			continue
		}
		clean[k] = v
		total += v
	}
	return DiagnosticGroup{Total: total, Details: clean}
}

// SetDiagnostics 写入当前任务诊断快照。
func (s *State) SetDiagnostics(diagnostics TaskDiagnostics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.diagnostics = diagnostics.Clone()
}

// AppendLog 追加日志，最多保留 500 条，同时写入日志文件
func (s *State) AppendLog(msg string) {
	s.logsMu.Lock()
	defer s.logsMu.Unlock()

	// 内存日志（保持现有逻辑）
	s.logs = append(s.logs, msg)
	if len(s.logs) > 500 {
		s.logs = s.logs[len(s.logs)-500:]
	}

	// 文件日志：写入失败不影响任务执行，降级为仅内存日志
	if s.logFile != nil {
		s.logFile.Write([]byte(msg))
	}
}

// InitLogFile 初始化日志文件写入器（在任务启动时调用）
func (s *State) InitLogFile(outputDir string) error {
	s.logsMu.Lock()
	defer s.logsMu.Unlock()

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	s.logFile = &lumberjack.Logger{
		Filename:   filepath.Join(outputDir, "app.log"),
		MaxSize:    64,    // 单文件最大 64MB
		MaxBackups: 1,     // 保留 1 个备份文件（当前文件 + 1 个轮转备份）
		MaxAge:     0,     // 不按时间清理
		Compress:   false, // 不压缩
		LocalTime:  true,  // 使用本地时间
	}
	return nil
}

// CloseLogFile 关闭日志文件写入器（在任务结束时调用）
func (s *State) CloseLogFile() error {
	s.logsMu.Lock()
	defer s.logsMu.Unlock()

	if s.logFile != nil {
		err := s.logFile.Close()
		s.logFile = nil
		return err
	}
	return nil
}

// GetLogs 获取所有当前日志记录的副本
func (s *State) GetLogs() []string {
	s.logsMu.Lock()
	defer s.logsMu.Unlock()
	logs := make([]string, len(s.logs))
	copy(logs, s.logs)
	return logs
}

// GetStatus 获取当前并发状态 (结构与之前 GetStatus() map 保持一致)
func (s *State) GetStatus() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	elapsed := 0.0
	if s.running && !s.startTime.IsZero() {
		elapsed = time.Since(s.startTime).Seconds()
	}

	return map[string]interface{}{
		"running":              s.running,
		"total":                s.total,
		"completed":            s.completed,
		"success":              s.success,
		"failed":               s.failed,
		"successTarget":        s.successTarget,
		"successTargetEnabled": s.successTargetEnabled,
		"elapsed":              elapsed,
		"diagnostics":          s.diagnostics.Clone(),
	}
}
