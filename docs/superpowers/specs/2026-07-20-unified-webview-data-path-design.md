# 统一 WebView2 数据目录

## 目标

Windows 桌面构建与 `start.ps1` 开发模式固定使用同一个 WebView2 用户数据目录：

`%APPDATA%\kirox\webview2`

## 设计

- 在 Windows 平台选项中设置 Wails 的 `WebviewUserDataPath`。
- 目录基于 `os.UserConfigDir()` 生成；解析失败时留空，沿用 Wails 默认行为，避免应用无法启动。
- 不改动现有 `%APPDATA%\kirox\storage.conf`、渠道统计及其他业务数据路径。
- 不迁移 `%APPDATA%\kirox.exe` 或 `%APPDATA%\kirox-dev.exe` 中旧的 WebView2 缓存；主题等纯浏览器状态会在统一目录首次启动时重新生成。
- 两种启动方式不同时运行，因此不增加单实例或跨进程文件锁。

## 验证

- 单元测试验证 Windows 平台选项生成固定的统一路径。
- 运行完整 Go 测试。
- 构建 Windows 可执行文件，确认配置可编译。
