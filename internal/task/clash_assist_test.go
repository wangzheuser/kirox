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

	if !shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://127.0.0.1:7890", "http://127.0.0.1:7890", cfg, 1) {
		t.Fatalf("serial local normal proxy using the Clash endpoint should use Clash-assisted realism")
	}
	if shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://127.0.0.1:7891", "http://127.0.0.1:7890", cfg, 2) {
		t.Fatalf("normal Clash assist should stay disabled for concurrent runs because Clash selector is global")
	}
	if shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://127.0.0.1:7891", "http://127.0.0.1:7890", cfg, 1) {
		t.Fatalf("different local proxy endpoints must not be treated as the same Clash egress")
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

func TestShouldEnableNormalClashAssistForLocalProxyTemplate(t *testing.T) {
	cfg := proxy.ClashConfig{Enabled: true, APIURL: "http://127.0.0.1:9097"}

	if shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://session-{uuid}:secret@127.0.0.1:9200", "http://127.0.0.1:7890", cfg, 1) {
		t.Fatalf("local normal proxy templates on a different port must not inherit Clash locale; their egress may be independent")
	}
}

func TestShouldEnableNormalClashAssistWhenNormalProxyIsSameLocalClashEndpoint(t *testing.T) {
	cfg := proxy.ClashConfig{Enabled: true, APIURL: "http://127.0.0.1:9097"}

	if !shouldEnableNormalClashAssist(storage.ProxyModeNormal, "http://session-{uuid}:secret@127.0.0.1:7890", "http://127.0.0.1:7890", cfg, 1) {
		t.Fatalf("normal mode may use Clash-assisted locale only when the normal proxy endpoint is the configured Clash endpoint")
	}
}

func TestApplyClashSelectionToConfigBindsFingerprintAndLocale(t *testing.T) {
	cfg := core.NewConfig()
	cfg.Proxy = "http://127.0.0.1:7891"

	locale := applyClashSelectionToConfigForSubject(cfg, cfg.Proxy, proxy.ClashSelection{Node: "日本东京3-AN | 1x"}, "normal-clash:", "")

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

func TestApplyClashSelectionToConfigForSubjectSeparatesAccountFingerprints(t *testing.T) {
	selection := proxy.ClashSelection{Node: "日本东京3-AN | 1x"}
	cfgA := core.NewConfig()
	cfgB := core.NewConfig()
	cfgRetry := core.NewConfig()

	applyClashSelectionToConfigForSubject(cfgA, "http://127.0.0.1:7890", selection, "clash:", "alice@example.com")
	applyClashSelectionToConfigForSubject(cfgB, "http://127.0.0.1:7890", selection, "clash:", "bob@example.com")
	applyClashSelectionToConfigForSubject(cfgRetry, "http://127.0.0.1:7890", selection, "clash:", "ALICE@example.com")

	if cfgA.FingerprintKey == cfgB.FingerprintKey {
		t.Fatalf("different accounts on the same node should not reuse one hardware fingerprint key: %q", cfgA.FingerprintKey)
	}
	if cfgA.FingerprintKey != cfgRetry.FingerprintKey {
		t.Fatalf("same account retry should keep a stable fingerprint key: %q vs %q", cfgA.FingerprintKey, cfgRetry.FingerprintKey)
	}
	if strings.Contains(cfgA.FingerprintKey, "alice@example.com") {
		t.Fatalf("fingerprint key should not store raw account email: %q", cfgA.FingerprintKey)
	}
}

func TestApplyRuntimeProxyCountryLocaleToConfigForSubjectKeepsDefaultLocale(t *testing.T) {
	cfg := core.NewConfig()
	cfg.Proxy = "http://127.0.0.1:9200"
	defaultAccept := cfg.AcceptLanguage
	defaultI18Next := cfg.I18Next
	defaultTimeZone := cfg.TimeZone

	locale, ok := applyRuntimeProxyCountryLocaleToConfigForSubject(cfg, "US", "alice@example.com")
	if ok {
		t.Fatalf("runtime proxy country should be observed but not override browser locale, got locale=%+v", locale)
	}
	if cfg.I18Next != defaultI18Next || cfg.AcceptLanguage != defaultAccept || cfg.TimeZone != defaultTimeZone || !cfg.TimeZoneSet {
		t.Fatalf("runtime country should not change browser locale, got accept=%q i18next=%q tz=%d set=%v", cfg.AcceptLanguage, cfg.I18Next, cfg.TimeZone, cfg.TimeZoneSet)
	}
	if cfg.FingerprintKey != "" {
		t.Fatalf("runtime country should not change fingerprint key, got %q", cfg.FingerprintKey)
	}
}

func TestNormalTemplateFingerprintKeyIsStableAndDoesNotLeakSecrets(t *testing.T) {
	proxyA := "http://Default.11111111-1111-1111-1111-111111111111:admin2012@127.0.0.1:9200"
	proxyB := "http://Default.22222222-2222-2222-2222-222222222222:admin2012@127.0.0.1:9200"

	keyA := normalTemplateFingerprintKey(proxyA, "alice@example.com")
	keyB := normalTemplateFingerprintKey(proxyB, "bob@example.com")
	keyRetry := normalTemplateFingerprintKey(proxyA, "ALICE@example.com")

	if keyA == keyB {
		t.Fatalf("different rendered template proxies and accounts should not share one hardware fingerprint key: %q", keyA)
	}
	if keyA != keyRetry {
		t.Fatalf("same rendered proxy and account retry should keep a stable fingerprint key: %q vs %q", keyA, keyRetry)
	}
	for _, secret := range []string{"alice@example.com", "admin2012", "11111111-1111-1111-1111-111111111111"} {
		if strings.Contains(keyA, secret) {
			t.Fatalf("fingerprint key should not leak %q: %q", secret, keyA)
		}
	}
	if !strings.HasPrefix(keyA, normalTemplateFingerprintPrefix) {
		t.Fatalf("fingerprint key should use normal template prefix, got %q", keyA)
	}
}
