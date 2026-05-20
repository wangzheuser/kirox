package storage

import "testing"

func TestPageStayConfigDefaultsToCurrentRange(t *testing.T) {
	withTempStorageConfig(t, "")

	got := GetPageStayConfig()
	if got.MinMs != DefaultPageStayMinMs || got.MaxMs != DefaultPageStayMaxMs {
		t.Fatalf("默认页面停留配置错误: got %+v", got)
	}
}

func TestPageStayConfigAllowsZeroRange(t *testing.T) {
	withTempStorageConfig(t, "")

	if err := SetPageStayConfig(PageStayConfig{MinMs: 0, MaxMs: 0}); err != nil {
		t.Fatalf("0/0 应允许表示不延迟: %v", err)
	}
	got := GetPageStayConfig()
	if got.MinMs != 0 || got.MaxMs != 0 {
		t.Fatalf("0/0 配置读取错误: got %+v", got)
	}
}

func TestPageStayConfigAllowsFixedRange(t *testing.T) {
	withTempStorageConfig(t, "")

	if err := SetPageStayConfig(PageStayConfig{MinMs: 3000, MaxMs: 3000}); err != nil {
		t.Fatal(err)
	}
	got := GetPageStayConfig()
	if got.MinMs != 3000 || got.MaxMs != 3000 {
		t.Fatalf("固定页面停留配置读取错误: got %+v", got)
	}
}

func TestPageStayConfigRejectsInvalidRange(t *testing.T) {
	withTempStorageConfig(t, "")

	if err := SetPageStayConfig(PageStayConfig{MinMs: 9000, MaxMs: 3000}); err == nil {
		t.Fatalf("min > max 应返回错误")
	}
	if err := SetPageStayConfig(PageStayConfig{MinMs: -1, MaxMs: 3000}); err == nil {
		t.Fatalf("负数页面停留时间应返回错误")
	}
}
