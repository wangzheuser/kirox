# 日志文件持久化设计文档

**作者：** wangqiupei  
**日期：** 2026-05-29  
**状态：** 待审查

## 1. 需求概述

当前项目的日志仅存在于内存（最近 500 条）和控制台输出，程序关闭后日志丢失。需要添加日志文件持久化功能，满足以下要求：

- 日志文件保存在结果输出目录（与 `storage.GetResultOutputDir()` 相同）
- 单文件自动轮转，文件大小限制 64M
- 实时写入（lumberjack 内部缓冲，约 4KB 或数秒后自动刷盘）
- 固定文件名 `app.log`
- 内存日志保持现状（500 条），与文件日志独立

## 2. 技术方案

### 2.1 方案选择

采用 **lumberjack** 库实现日志轮转和大小限制：

- **库：** `gopkg.in/natefinch/lumberjack.v2`
- **理由：** 成熟稳定、自动处理并发安全、代码量少
- **配置：** `MaxSize: 64MB`，`MaxBackups: 0`（不保留备份）

### 2.2 架构设计

**数据流：**

```
log.Printf()
  ↓
logWriter.Write()
  ├→ task.Manager.AppendLog() (内存，最近 500 条)
  │   └→ lumberjack.Logger.Write() (文件，64M 轮转)
  └→ os.Stderr.Write() (控制台输出)
```

**核心变更：**

1. `task.State` 增加 `logFile *lumberjack.Logger` 字段
2. `AppendLog()` 方法同时写入内存和文件
3. 任务启动时初始化日志文件，任务结束时关闭

## 3. 详细设计

### 3.1 数据结构

**`internal/task/state.go`**

```go
type State struct {
    mu         sync.Mutex
    running    bool
    stopCh     chan struct{}
    cancelFunc context.CancelFunc
    total      int
    completed  int
    success    int
    failed     int
    results    []map[string]interface{}
    startTime  time.Time
    logs       []string
    logsMu     sync.Mutex
    
    // 新增字段
    logFile *lumberjack.Logger  // 日志文件写入器
}
```

### 3.2 核心方法

**初始化日志文件：**

```go
// InitLogFile 初始化日志文件（在任务启动时调用）
func (s *State) InitLogFile(outputDir string) error {
    s.logsMu.Lock()
    defer s.logsMu.Unlock()
    
    // 确保目录存在
    if err := os.MkdirAll(outputDir, 0755); err != nil {
        return err
    }
    
    logPath := filepath.Join(outputDir, "app.log")
    
    s.logFile = &lumberjack.Logger{
        Filename:   logPath,
        MaxSize:    64,        // MB
        MaxBackups: 0,         // 不保留备份（lumberjack 会自动清理）
        MaxAge:     0,         // 不按时间清理
        Compress:   false,     // 不压缩
        LocalTime:  true,      // 使用本地时间
    }
    
    return nil
}
```

**关闭日志文件：**

```go
// CloseLogFile 关闭日志文件（在任务结束时调用）
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
```

**修改日志追加方法：**

```go
// AppendLog 追加日志到内存和文件
func (s *State) AppendLog(msg string) {
    s.logsMu.Lock()
    defer s.logsMu.Unlock()
    
    // 内存日志（保持现有逻辑）
    s.logs = append(s.logs, msg)
    if len(s.logs) > 500 {
        s.logs = s.logs[len(s.logs)-500:]
    }
    
    // 文件日志（新增）
    if s.logFile != nil {
        s.logFile.Write([]byte(msg))
    }
}
```

**`app.go` 修改：**

```go
// logWriter.Write 保持不变，仍然调用 AppendLog
func (w *logWriter) Write(p []byte) (int, error) {
    msg := string(p)
    task.Manager.AppendLog(msg)
    return os.Stderr.Write(p)
}
```

### 3.3 调用时机

**任务启动时（`internal/task/coordinator.go` 的 `startTask` 函数）：**

```go
func startTask(req StartTaskRequest) map[string]interface{} {
    // ... 现有代码 ...
    
    // 初始化日志文件
    outDir := req.OutputPath
    if outDir == "" {
        outDir = storage.GetResultOutputDir()
    }
    if err := Manager.InitLogFile(outDir); err != nil {
        log.Printf("[Kiro] 日志文件初始化失败，降级为仅内存日志: %v", err)
    }
    
    // 后台执行
    go runBatch(req, emailProvider, outlookAccounts)
    
    return map[string]interface{}{"status": "started"}
}
```

**任务结束时（`runBatch` 函数末尾）：**

```go
func runBatch(req StartTaskRequest, emailProvider string, outlookAccounts []email.OutlookAccount) {
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
    
    // ... 现有任务逻辑 ...
}
```

## 4. 错误处理

### 4.1 初始化失败

**场景：** 目录不存在、权限不足、磁盘空间不足

**处理：**
- 记录错误到 stderr
- 降级为仅内存日志，不阻断任务启动
- 在控制台输出警告信息

### 4.2 写入失败

**场景：** 磁盘满、文件被锁定、权限变更

**处理：**
- lumberjack 内部会尝试重新打开文件
- 如果持续失败，日志会丢失但不影响任务执行
- 控制台输出（`os.Stderr`）仍然正常

### 4.3 文件轮转

**lumberjack 自动处理：**
1. 检测到文件大小超过 64M
2. 关闭当前 `app.log`
3. 重命名为 `app.log.1`
4. 创建新的 `app.log`
5. 由于 `MaxBackups: 0`，自动删除 `app.log.1`

**磁盘占用：**
- 稳定状态：最多 64MB（单个 `app.log`）
- 轮转瞬间：可能短暂达到 128MB（`app.log` + `app.log.1`）

### 4.4 并发安全

- lumberjack 内部已加锁，支持并发写入
- `State.AppendLog()` 使用 `logsMu` 保护内存日志
- 无需额外同步机制

## 5. 测试策略

### 5.1 单元测试

**`internal/task/state_test.go`**

```go
// TestInitLogFile 验证日志文件创建
func TestInitLogFile(t *testing.T) {
    tmpDir := t.TempDir()
    state := &State{}
    
    err := state.InitLogFile(tmpDir)
    if err != nil {
        t.Fatalf("InitLogFile failed: %v", err)
    }
    
    logPath := filepath.Join(tmpDir, "app.log")
    if _, err := os.Stat(logPath); os.IsNotExist(err) {
        t.Fatalf("log file not created")
    }
}

// TestLogFileRotation 验证文件大小限制
func TestLogFileRotation(t *testing.T) {
    tmpDir := t.TempDir()
    state := &State{}
    state.InitLogFile(tmpDir)
    defer state.CloseLogFile()
    
    // 写入 70M 数据
    largeMsg := strings.Repeat("x", 1024*1024) // 1MB
    for i := 0; i < 70; i++ {
        state.AppendLog(largeMsg)
    }
    
    logPath := filepath.Join(tmpDir, "app.log")
    info, _ := os.Stat(logPath)
    if info.Size() > 64*1024*1024 {
        t.Fatalf("log file exceeds 64MB: %d bytes", info.Size())
    }
}

// TestLogFileConcurrency 验证并发安全
func TestLogFileConcurrency(t *testing.T) {
    tmpDir := t.TempDir()
    state := &State{}
    state.InitLogFile(tmpDir)
    defer state.CloseLogFile()
    
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            state.AppendLog(fmt.Sprintf("log-%d\n", id))
        }(i)
    }
    wg.Wait()
}
```

### 5.2 集成测试

**手动验证步骤：**

1. 启动任务，观察 `app.log` 文件创建
2. 运行长时间任务，生成大量日志
3. 验证文件大小不超过 64M
4. 检查 `app.log.1` 备份文件在下次启动时被清理
5. 对比内存日志（前端展示）与文件日志内容一致性

## 6. 依赖管理

**添加依赖：**

```bash
go get gopkg.in/natefinch/lumberjack.v2
```

**`go.mod` 变更：**

```
require (
    gopkg.in/natefinch/lumberjack.v2 v2.2.1
)
```

## 7. 性能影响

**写入性能：**
- 每条日志增加一次文件 I/O
- lumberjack 内部有约 4KB 缓冲，数秒或缓冲满时自动刷盘
- 预计单条日志写入延迟 < 0.1ms（内存缓冲）
- 对任务整体性能影响可忽略（日志非热路径）

**注意：** 如果程序崩溃，缓冲区中的日志（最后数秒）可能丢失。如需绝对可靠性，可在 `AppendLog()` 中每次调用 `logFile.Sync()` 强制刷盘，但会降低性能（每条日志约 1-5ms）。

**内存占用：**
- lumberjack 内部缓冲约 4KB
- 内存日志保持 500 条不变
- 总增加内存 < 10KB

**磁盘占用：**
- 稳定状态：最多 64MB
- 轮转瞬间：可能短暂达到 128MB

## 8. 向后兼容性

**现有功能不受影响：**
- 内存日志（`GetLogs()`）保持 500 条限制
- 控制台输出（`os.Stderr`）正常
- 前端实时日志展示无变化

**新增功能：**
- 用户可在结果目录找到 `app.log` 文件
- 支持离线查看完整日志历史

## 9. 未来扩展

**可选增强（本次不实现）：**

1. **日志级别过滤** — 仅记录 ERROR/WARN 到文件
2. **日志压缩** — 启用 lumberjack 的 `Compress: true`
3. **多文件分类** — 按任务 ID 或日期分文件
4. **前端查看** — 增加 API 读取日志文件内容
5. **日志搜索** — 支持关键词搜索历史日志

## 10. 风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 磁盘空间不足 | 日志写入失败 | 降级为仅内存日志，不阻断任务 |
| 文件权限问题 | 无法创建日志文件 | 初始化时检测并记录错误 |
| 并发写入冲突 | 数据竞争 | lumberjack 内部已加锁 |
| 日志文件过大 | 占用磁盘空间 | 64M 硬限制 + 自动轮转 |

## 11. 实施计划

**阶段 1：核心实现**
1. 添加 lumberjack 依赖
2. 修改 `internal/task/state.go`
3. 修改 `internal/task/coordinator.go`

**阶段 2：测试验证**
1. 编写单元测试
2. 手动集成测试
3. 性能基准测试

**阶段 3：文档和发布**
1. 更新 README 说明日志文件位置
2. 提交代码并创建 PR
3. 发布版本说明

## 12. 总结

本设计采用成熟的 lumberjack 库实现日志文件持久化，满足所有需求：

- ✅ 保存在结果输出目录
- ✅ 单文件自动轮转，64M 限制
- ✅ 实时写入（内部缓冲，数秒后刷盘）
- ✅ 固定文件名 `app.log`
- ✅ 内存日志独立保持 500 条

实现简单（约 40 行新增代码）、性能影响小、向后兼容，风险可控。
