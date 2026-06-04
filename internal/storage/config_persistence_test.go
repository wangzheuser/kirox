package storage

import (
	"strings"
	"sync"
	"testing"

	"reg_go/internal/proxy"
)

func TestConfigStorageRoundTripsAllPersistentSettings(t *testing.T) {
	withTempStorageConfig(t, "legacy_keep=value\noutlook_register_domain_override=outlook.fr\n")

	dataDir := t.TempDir()
	resultDir := t.TempDir()

	if _, err := SetDataDirPath(dataDir); err != nil {
		t.Fatalf("SetDataDirPath returned error: %v", err)
	}
	if _, err := SetResultOutputDir(resultDir); err != nil {
		t.Fatalf("SetResultOutputDir returned error: %v", err)
	}
	if err := SetPageStayConfig(PageStayConfig{MinMs: 0, MaxMs: 0}); err != nil {
		t.Fatalf("SetPageStayConfig returned error: %v", err)
	}
	if err := SetOutlookScope(OutlookScopeGraph); err != nil {
		t.Fatalf("SetOutlookScope returned error: %v", err)
	}
	if err := SetProxyMode(ProxyModeClash); err != nil {
		t.Fatalf("SetProxyMode returned error: %v", err)
	}
	if _, err := SetProxy("http://127.0.0.1:7890"); err != nil {
		t.Fatalf("SetProxy returned error: %v", err)
	}
	if _, err := SetClashProxy("127.0.0.1:7890"); err != nil {
		t.Fatalf("SetClashProxy returned error: %v", err)
	}
	if _, err := SetEmailProxy("127.0.0.1:7891"); err != nil {
		t.Fatalf("SetEmailProxy returned error: %v", err)
	}
	if err := SetClashConfig(proxy.ClashConfig{
		Enabled:              true,
		APIURL:               "http://127.0.0.1:9097",
		APISecret:            "secret",
		ProxyGroup:           "Proxy",
		TestURL:              "https://example.com/ping",
		TestTimeout:          7,
		SkipConnectivityTest: true,
	}); err != nil {
		t.Fatalf("SetClashConfig returned error: %v", err)
	}
	if err := SetKillSwitchEnabled(false); err != nil {
		t.Fatalf("SetKillSwitchEnabled returned error: %v", err)
	}
	if err := SetSoundEnabled(false); err != nil {
		t.Fatalf("SetSoundEnabled returned error: %v", err)
	}
	if err := SetVerifyModelsEnabled(true); err != nil {
		t.Fatalf("SetVerifyModelsEnabled returned error: %v", err)
	}
	if err := SetRegistrationConfig(RegistrationConfig{
		Count:             7,
		Concurrency:       3,
		Delay:             0,
		EmailProvider:     "moemail",
		MoeMailDomainMode: "custom",
		MoeMailDomains:    []string{"alpha.example", "beta.example"},
	}); err != nil {
		t.Fatalf("SetRegistrationConfig returned error: %v", err)
	}

	if got := GetDataDir(); got != dataDir {
		t.Fatalf("data_dir round-trip failed: got %q, want %q", got, dataDir)
	}
	if got := GetResultOutputDir(); got != resultDir {
		t.Fatalf("result_output_dir round-trip failed: got %q, want %q", got, resultDir)
	}
	if got := GetPageStayConfig(); got.MinMs != 0 || got.MaxMs != 0 {
		t.Fatalf("page stay 0/0 round-trip failed: got %+v", got)
	}
	if got := GetOutlookScope(); got != OutlookScopeGraph {
		t.Fatalf("outlook_scope round-trip failed: got %q", got)
	}
	if got := GetProxyMode(); got != ProxyModeClash {
		t.Fatalf("proxy_mode round-trip failed: got %q", got)
	}
	if got := GetProxy(); got != "http://127.0.0.1:7890" {
		t.Fatalf("proxy round-trip failed: got %q", got)
	}
	if got := GetClashProxy(); got != "socks5://127.0.0.1:7890" {
		t.Fatalf("clash_proxy round-trip failed: got %q", got)
	}
	if got := GetEmailProxy(); got != "socks5://127.0.0.1:7891" {
		t.Fatalf("email_proxy round-trip failed: got %q", got)
	}
	if got := GetClashConfig(); !got.Enabled || got.APISecret != "secret" || got.ProxyGroup != "Proxy" || got.TestTimeout != 7 || !got.SkipConnectivityTest {
		t.Fatalf("clash config round-trip failed: got %+v", got)
	}
	if got := GetKillSwitchEnabled(); got {
		t.Fatalf("kill switch false should persist")
	}
	if got := GetSoundEnabled(); got {
		t.Fatalf("sound false should persist")
	}
	if got := GetVerifyModelsEnabled(); !got {
		t.Fatalf("verify models true should persist")
	}
	if got := GetRegistrationConfig(); got.Count != 7 || got.Concurrency != 3 || got.Delay != 0 || got.EmailProvider != "moemail" || got.MoeMailDomainMode != "custom" || strings.Join(got.MoeMailDomains, ",") != "alpha.example,beta.example" || !got.Saved {
		t.Fatalf("registration config round-trip failed: got %+v", got)
	}

	saved := loadConfigMap()
	if saved["legacy_keep"] != "value" {
		t.Fatalf("unknown config keys should be preserved, got %q", saved["legacy_keep"])
	}
	if _, ok := saved["outlook_register_domain_override"]; ok {
		t.Fatalf("deprecated outlook_register_domain_override should not be written back")
	}
}

func TestConfigStorageSerializesConcurrentSettersWithoutDroppingKeys(t *testing.T) {
	withTempStorageConfig(t, "")

	var wg sync.WaitGroup
	wg.Add(5)
	go func() {
		defer wg.Done()
		if _, err := SetProxy("https://user:pass@example.com:443"); err != nil {
			t.Errorf("SetProxy returned error: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := SetOutlookScope(OutlookScopeGraph); err != nil {
			t.Errorf("SetOutlookScope returned error: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := SetKillSwitchEnabled(false); err != nil {
			t.Errorf("SetKillSwitchEnabled returned error: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := SetSoundEnabled(false); err != nil {
			t.Errorf("SetSoundEnabled returned error: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := SetVerifyModelsEnabled(true); err != nil {
			t.Errorf("SetVerifyModelsEnabled returned error: %v", err)
		}
	}()
	wg.Wait()

	saved := loadConfigMap()
	for _, key := range []string{keyProxy, keyOutlookScope, keyKillSwitchEnabled, keySoundEnabled, keyVerifyModelsEnabled} {
		if _, ok := saved[key]; !ok {
			t.Fatalf("concurrent setters dropped %s; saved config: %#v", key, saved)
		}
	}
}

func TestVerifyModelsEnabledDefaultsOffAndPersists(t *testing.T) {
	withTempStorageConfig(t, "")

	if got := GetVerifyModelsEnabled(); got {
		t.Fatalf("二次模型验活默认应关闭")
	}
	if err := SetVerifyModelsEnabled(true); err != nil {
		t.Fatalf("SetVerifyModelsEnabled(true) returned error: %v", err)
	}
	if got := GetVerifyModelsEnabled(); !got {
		t.Fatalf("二次模型验活开启后应读取为 true")
	}
	if err := SetVerifyModelsEnabled(false); err != nil {
		t.Fatalf("SetVerifyModelsEnabled(false) returned error: %v", err)
	}
	if got := GetVerifyModelsEnabled(); got {
		t.Fatalf("二次模型验活关闭后应读取为 false")
	}
}

func TestRegistrationConfigDefaultsAndValidation(t *testing.T) {
	withTempStorageConfig(t, "")

	defaults := GetRegistrationConfig()
	if defaults.Count != 1 || defaults.Concurrency != 1 || defaults.Delay != 1 || defaults.EmailProvider != "outlook" || defaults.MoeMailDomainMode != "random" || defaults.Saved {
		t.Fatalf("registration defaults mismatch: got %+v", defaults)
	}

	if err := SetRegistrationConfig(RegistrationConfig{Count: 0, Concurrency: 1, Delay: 0, EmailProvider: "outlook"}); err == nil {
		t.Fatalf("count < 1 should be rejected")
	}
	if err := SetRegistrationConfig(RegistrationConfig{Count: 1, Concurrency: 0, Delay: 0, EmailProvider: "outlook"}); err == nil {
		t.Fatalf("concurrency < 1 should be rejected")
	}
	if err := SetRegistrationConfig(RegistrationConfig{Count: 1, Concurrency: 1, Delay: -1, EmailProvider: "outlook"}); err == nil {
		t.Fatalf("negative delay should be rejected")
	}
	if err := SetRegistrationConfig(RegistrationConfig{Count: 1, Concurrency: 1, Delay: 0, EmailProvider: "invalid"}); err == nil {
		t.Fatalf("invalid email provider should be rejected")
	}
}
