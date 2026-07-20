//go:build windows
// +build windows

package main

import (
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// getPlatformOptions 返回 Windows 平台特定选项
func getPlatformOptions() []func(*options.App) {
	configDir, _ := os.UserConfigDir()
	webviewUserDataPath := ""
	if configDir != "" {
		webviewUserDataPath = filepath.Join(configDir, "kirox", "webview2")
	}

	return []func(*options.App){
		func(app *options.App) {
			app.Windows = &windows.Options{
				WebviewIsTransparent: false,
				WindowIsTranslucent:  false,
				DisableWindowIcon:    false,
				WebviewUserDataPath:  webviewUserDataPath,
			}
		},
	}
}
