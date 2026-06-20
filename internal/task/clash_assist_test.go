package task

import (
	"strings"
	"testing"

	"reg_go/internal/core"
	"reg_go/internal/proxy"
	"reg_go/internal/storage"
)

func TestShouldEnableNormalClashAssistOnlyForSerialLocalNormalProxy(t *testing.T) {
	cfg := proxy.ClashConfig{Enabled: true, APIURL: "http://127.0.0.1:9097"}

	if !shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://127.0.0.1:7891", "http://127.0.0.1:7890", cfg, 1) {
		t.Fatalf("serial local normal proxy with enabled Clash API should use Clash-assisted realism")
	}
	if shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://127.0.0.1:7891", "http://127.0.0.1:7890", cfg, 2) {
		t.Fatalf("normal Clash assist should stay disabled for concurrent runs because Clash selector is global")
	}
	if shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://198.51.100.10:8080", "http://127.0.0.1:7890", cfg, 1) {
		t.Fatalf("remote normal proxies should not inherit local Clash node metadata")
	}
	if shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://127.0.0.1:7891", "http://127.0.0.1:7890", proxy.ClashConfig{}, 1) {
		t.Fatalf("disabled Clash config should not enable assist")
	}
	if shouldEnableNormalClashAssist(storage.ProxyModeClash, "http://127.0.0.1:7891", "http://127.0.0.1:7890", cfg, 1) {
		t.Fatalf("explicit Clash mode uses the dedicated Clash path, not normal assist")
	}
}

func TestApplyClashSelectionToConfigBindsFingerprintAndLocale(t *testing.T) {
	cfg := core.NewConfig()
	cfg.Proxy = "http://127.0.0.1:7891"

	locale := applyClashSelectionToConfig(cfg, cfg.Proxy, proxy.ClashSelection{Node: "日本东京3-AN | 1x"}, "normal-clash:")

	if cfg.Proxy != "http://127.0.0.1:7891" {
		t.Fatalf("proxy URL should stay on the configured normal proxy, got %q", cfg.Proxy)
	}
	if cfg.FingerprintKey != "normal-clash:日本东京3-AN | 1x" {
		t.Fatalf("fingerprint key should be bound to the selected node, got %q", cfg.FingerprintKey)
	}
	if !strings.HasPrefix(cfg.AcceptLanguage, "ja-JP") || cfg.I18Next != "ja-JP" || cfg.TimeZone != 9 || !cfg.TimeZoneSet {
		t.Fatalf("expected Japan browser locale, got accept=%q i18next=%q tz=%d set=%v", cfg.AcceptLanguage, cfg.I18Next, cfg.TimeZone, cfg.TimeZoneSet)
	}
	if !cfg.ProxySwitchable {
		t.Fatalf("Clash-assisted normal proxy should be marked switchable for network retry handling")
	}
	if locale.I18Next != "ja-JP" || locale.TimeZone != 9 {
		t.Fatalf("returned locale should match applied locale, got %+v", locale)
	}
}
