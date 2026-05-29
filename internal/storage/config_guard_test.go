package storage

import (
	"errors"
	"os"
	"testing"
)

// TestModifyConfigMapAbortsOnReadError 验证：当读取现有配置失败时（瞬时文件占用等），
// modifyConfigMap 必须中止保存，绝不能用不完整的内存数据覆盖磁盘上的完整配置。
// 这是防止"单次读抖动清空全部配置"的核心保障。
func TestModifyConfigMapAbortsOnReadError(t *testing.T) {
	withTempStorageConfig(t, "proxy=http://user:pass@example.com:443\nemail_proxy=socks5://127.0.0.1:7891\n")

	// 注入一次读失败，模拟文件被杀软/同步工具瞬时锁定
	injectErr := errors.New("模拟读取失败：文件被占用")
	orig := readConfigFile
	readConfigFile = func(string) ([]byte, error) { return nil, injectErr }
	t.Cleanup(func() { readConfigFile = orig })

	// 在读失败状态下尝试写入一个配置项
	_, err := SetEmailProxy("127.0.0.1:9999")
	readConfigFile = orig // 立即恢复，便于后续断言读取真实文件

	if err == nil {
		t.Fatalf("读失败时 SetEmailProxy 应返回错误并中止保存，却返回 nil")
	}

	// 磁盘上的原始完整配置必须原封不动
	saved := loadConfigMap()
	if saved["proxy"] != "http://user:pass@example.com:443" {
		t.Fatalf("读失败导致 proxy 被清空，saved=%#v", saved)
	}
	if saved["email_proxy"] != "socks5://127.0.0.1:7891" {
		t.Fatalf("读失败不应改动 email_proxy，saved=%#v", saved)
	}
}

// TestModifyConfigMapWorksWhenFileAbsent 验证：配置文件不存在是合法初始状态，
// 此时 modifyConfigMap 应正常写入新键，而不是误判为读失败。
func TestModifyConfigMapWorksWhenFileAbsent(t *testing.T) {
	withTempStorageConfig(t, "")
	// 删除文件，制造"全新安装"场景
	if err := os.Remove(getConfigFilePath()); err != nil {
		t.Fatalf("删除配置文件失败: %v", err)
	}

	if _, err := SetEmailProxy("127.0.0.1:7891"); err != nil {
		t.Fatalf("文件不存在时写入应成功，却返回错误: %v", err)
	}
	if got := GetEmailProxy(); got != "socks5://127.0.0.1:7891" {
		t.Fatalf("文件不存在时写入未生效: got %q", got)
	}
}
