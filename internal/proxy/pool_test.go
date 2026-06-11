package proxy

import "testing"

func TestPickRandomSkipsDisabledEntries(t *testing.T) {
	InitPool(t.TempDir())

	disabled, err := Add(PoolEntry{URL: "http://disabled.proxy.test:8080", Weight: 100})
	if err != nil {
		t.Fatalf("新增禁用候选代理失败: %v", err)
	}
	enabled, err := Add(PoolEntry{URL: "http://enabled.proxy.test:8080", Weight: 1})
	if err != nil {
		t.Fatalf("新增启用候选代理失败: %v", err)
	}
	if _, err := Update(disabled.ID, PoolEntry{Enabled: false}); err != nil {
		t.Fatalf("禁用候选代理失败: %v", err)
	}

	// 多次抽样确认高权重禁用项不会参与抽签。
	for i := 0; i < 20; i++ {
		if got := PickRandom(); got != enabled.URL {
			t.Fatalf("抽签不应命中禁用代理: got %q, want %q", got, enabled.URL)
		}
	}
}
