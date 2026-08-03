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

func TestProxyPoolQuarantinesAfterThreeNetworkFailures(t *testing.T) {
	InitPool(t.TempDir())
	_, err := Add(PoolEntry{Name: "bad", URL: "http://bad.example:8080", Weight: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Add(PoolEntry{Name: "good", URL: "http://good.example:8080", Weight: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}

	RecordPoolProxyNetworkFailure("http://bad.example:8080")
	RecordPoolProxyNetworkFailure("http://bad.example:8080")
	if QuarantinedPoolProxyCount() != 0 {
		t.Fatalf("proxy should not be quarantined before third failure")
	}
	RecordPoolProxyNetworkFailure("http://bad.example:8080")
	if QuarantinedPoolProxyCount() != 1 {
		t.Fatalf("proxy should be quarantined after third failure")
	}
	for i := 0; i < 20; i++ {
		if got := PickRandom(); got == "http://bad.example:8080" {
			t.Fatalf("quarantined proxy should be skipped, got %q", got)
		}
	}
	RecordPoolProxySuccess("http://bad.example:8080")
	if QuarantinedPoolProxyCount() != 0 {
		t.Fatalf("success should clear quarantine")
	}
}

func TestPickRandomEntryReturnsFalseWhenAllEntriesQuarantined(t *testing.T) {
	InitPool(t.TempDir())
	entry, err := Add(PoolEntry{Name: "only", URL: "http://only.example:8080", Weight: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < poolQuarantineFailures; i++ {
		RecordPoolProxyNetworkFailure(entry.URL)
	}
	if picked, ok := PickRandomEntry(); ok {
		t.Fatalf("all quarantined pool must not return a direct fallback or entry: %#v", picked)
	}
}
