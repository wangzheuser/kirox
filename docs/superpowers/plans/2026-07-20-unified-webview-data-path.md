# Unified WebView Data Path Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make packaged and development Windows builds use `%APPDATA%\kirox\webview2` as the same WebView2 user data directory.

**Architecture:** Configure Wails' existing `windows.Options.WebviewUserDataPath` in the Windows-only platform options. Derive the path from `os.UserConfigDir()` and leave it empty on lookup failure so Wails retains its fallback behavior.

**Tech Stack:** Go, Wails v2, Go standard library

---

### Task 1: Configure the shared Windows WebView2 directory

**Files:**
- Create: `main_windows_test.go`
- Modify: `main_windows.go`

- [ ] **Step 1: Write the failing test**

```go
//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/options"
)

func TestWindowsOptionsUseSharedWebviewDataPath(t *testing.T) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("UserConfigDir unavailable: %v", err)
	}

	appOpts := &options.App{}
	for _, apply := range getPlatformOptions() {
		apply(appOpts)
	}

	want := filepath.Join(configDir, "kirox", "webview2")
	if appOpts.Windows == nil || appOpts.Windows.WebviewUserDataPath != want {
		t.Fatalf("WebviewUserDataPath = %q, want %q", appOpts.Windows.WebviewUserDataPath, want)
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test . -run TestWindowsOptionsUseSharedWebviewDataPath -count=1`

Expected: FAIL because `WebviewUserDataPath` is empty.

- [ ] **Step 3: Add the minimal Windows configuration**

Add `os` and `path/filepath` imports to `main_windows.go`, derive the path once inside `getPlatformOptions`, and set the existing Wails option:

```go
configDir, _ := os.UserConfigDir()
webviewUserDataPath := ""
if configDir != "" {
	webviewUserDataPath = filepath.Join(configDir, "kirox", "webview2")
}
```

```go
WebviewUserDataPath: webviewUserDataPath,
```

- [ ] **Step 4: Run focused and full verification**

Run: `go test . -run TestWindowsOptionsUseSharedWebviewDataPath -count=1`

Expected: PASS.

Run: `go test ./...`

Expected: all packages PASS.

Run: `go build ./...`

Expected: exit code 0.

- [ ] **Step 5: Check task-related cleanup and commit**

Confirm no old WebView path constants or duplicate configuration were introduced, then run:

```powershell
git add -- main_windows.go main_windows_test.go docs/superpowers/plans/2026-07-20-unified-webview-data-path.md
git commit -m "fix: unify Windows WebView data path"
```
