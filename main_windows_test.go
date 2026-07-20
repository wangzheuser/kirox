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
