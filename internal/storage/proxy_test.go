package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNormalizeProxyAddressKeepsFullLocalURL(t *testing.T) {
	input := "http://127.0.0.1:7890"

	if got := NormalizeProxyAddress(input); got != input {
		t.Fatalf("完整本地代理 URL 不应被改写: got %q, want %q", got, input)
	}
}

func TestNormalizeProxyAddressSupportsHostPortUserPassTemplate(t *testing.T) {
	got := NormalizeProxyAddress("proxy.example.test:443:template-user:template-pass")
	want := "http://template-user:template-pass@proxy.example.test:443"

	if got != want {
		t.Fatalf("host:port:user:pass 模板归一化失败: got %q, want %q", got, want)
	}
}

func TestNormalizeProxyAddressKeepsHostPortDefaultBehavior(t *testing.T) {
	got := NormalizeProxyAddress("127.0.0.1:7890")
	want := "socks5://127.0.0.1:7890"

	if got != want {
		t.Fatalf("host:port 默认归一化行为不应回退: got %q, want %q", got, want)
	}
}

func TestProxyModeLegacyProxyDefaultsToNormal(t *testing.T) {
	withTempStorageConfig(t, "proxy=https://user:pass@example.com:443\n")

	if got := GetProxyMode(); got != ProxyModeNormal {
		t.Fatalf("旧普通代理配置应解释为 normal: got %q", got)
	}
}

func TestProxyModeLegacyClashLocalProxyDefaultsToClash(t *testing.T) {
	withTempStorageConfig(t, "proxy=http://127.0.0.1:7890\nclash_enabled=true\n")

	if got := GetProxyMode(); got != ProxyModeClash {
		t.Fatalf("旧 Clash 本地代理配置应解释为 clash: got %q", got)
	}
	if got := GetClashProxy(); got != "http://127.0.0.1:7890" {
		t.Fatalf("旧 Clash 本地代理应作为 clash_proxy 读取: got %q", got)
	}
}

func TestSetClashProxyDoesNotOverwriteNormalProxy(t *testing.T) {
	withTempStorageConfig(t, "")

	if _, err := SetProxy("http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	if _, err := SetClashProxy("http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	if got := GetProxy(); got != "http://127.0.0.1:7890" {
		t.Fatalf("普通代理不应被 Clash 代理覆盖: got %q", got)
	}
	if got := GetClashProxy(); got != "http://127.0.0.1:7890" {
		t.Fatalf("Clash 代理读取失败: got %q", got)
	}
}

func TestSetEmailProxyClearsWhenBlank(t *testing.T) {
	withTempStorageConfig(t, "email_proxy=http://127.0.0.1:7890\n")

	got, err := SetEmailProxy("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("清空邮箱代理应返回空字符串: got %q", got)
	}
	if stored := GetEmailProxy(); stored != "" {
		t.Fatalf("邮箱代理应已清空: got %q", stored)
	}
}

func TestSetEmailProxyNormalizesHostPort(t *testing.T) {
	withTempStorageConfig(t, "")

	got, err := SetEmailProxy("127.0.0.1:7890")
	if err != nil {
		t.Fatal(err)
	}
	if got != "socks5://127.0.0.1:7890" {
		t.Fatalf("邮箱代理 host:port 归一化失败: got %q", got)
	}
	if stored := GetEmailProxy(); stored != got {
		t.Fatalf("邮箱代理读取不一致: got %q, want %q", stored, got)
	}
}

func TestSetEmailProxyKeepsFullLocalURL(t *testing.T) {
	withTempStorageConfig(t, "")
	input := "http://127.0.0.1:7890"

	got, err := SetEmailProxy(input)
	if err != nil {
		t.Fatal(err)
	}
	if got != input {
		t.Fatalf("邮箱本地代理 URL 不应被改写: got %q, want %q", got, input)
	}
}

func withTempStorageConfig(t *testing.T, content string) {
	t.Helper()
	tempRoot := t.TempDir()
	for key, value := range storageTestEnvVars(tempRoot) {
		t.Setenv(key, value)
	}
	_dataDir = ""
	_dataDirOnce = sync.Once{}
	_resultOutputDir = ""
	_resultOutputOnce = sync.Once{}
	_proxy = ""
	_proxyOnce = sync.Once{}
	_killSwitchEnabled = false
	_killSwitchOnce = sync.Once{}
	_soundEnabled = false
	_soundOnce = sync.Once{}

	path := getConfigFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func storageTestEnvVars(tempRoot string) map[string]string {
	return map[string]string{
		"APPDATA":         tempRoot,
		"XDG_CONFIG_HOME": tempRoot,
		"HOME":            tempRoot,
		"USERPROFILE":     tempRoot,
	}
}

func TestStorageTestEnvVarsIncludeWindowsAppData(t *testing.T) {
	tmp := t.TempDir()
	env := storageTestEnvVars(tmp)
	if env["APPDATA"] != tmp {
		t.Fatalf("APPDATA must be isolated on Windows: %#v", env)
	}
	if env["XDG_CONFIG_HOME"] != tmp {
		t.Fatalf("XDG_CONFIG_HOME must be isolated on Unix: %#v", env)
	}
	if env["HOME"] != tmp {
		t.Fatalf("HOME must be isolated for fallback paths: %#v", env)
	}
}
