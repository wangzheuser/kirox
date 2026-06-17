package core

import (
	"testing"
	"time"
)

func TestBrowserLocaleForClashNodeUS(t *testing.T) {
	loc := BrowserLocaleForClashNode("🇺🇸 _洛杉矶-18")
	if loc.AcceptLanguage != "en-US,en;q=0.9" || loc.I18Next != "en-US" {
		t.Fatalf("US node locale mismatch: %+v", loc)
	}
}

func TestBrowserLocaleForClashNodeJapan(t *testing.T) {
	loc := BrowserLocaleForClashNode("🇯🇵【亚洲】日本01丨专线【3x】")
	if loc.AcceptLanguage != "ja-JP,ja;q=0.9,en;q=0.8" || loc.I18Next != "ja-JP" || loc.TimeZone != 9 {
		t.Fatalf("Japan node locale mismatch: %+v", loc)
	}
}

func TestBrowserLocaleDefaultIsCurrentChineseProfile(t *testing.T) {
	loc := BrowserLocaleForClashNode("")
	if loc.AcceptLanguage != "zh-CN,zh;q=0.9,en;q=0.8" || loc.I18Next != "zh-CN" || loc.TimeZone != 8 {
		t.Fatalf("default locale mismatch: %+v", loc)
	}
}

func TestRegistrarHeadersUseConfiguredBrowserLocale(t *testing.T) {
	cfg := NewConfig()
	cfg.AcceptLanguage = "en-US,en;q=0.9"
	cfg.I18Next = "en-US"
	cfg.TimeZone = -8
	cfg.TimeZoneSet = true
	reg := NewRegistrar(cfg)

	if got := reg.BuildHeaders("", "")["Accept-Language"]; got != "en-US,en;q=0.9" {
		t.Fatalf("BuildHeaders Accept-Language=%q", got)
	}
	if got := reg.BuildProfileHeaders("https://profile.aws.amazon.com/")["Accept-Language"]; got != "en-US,en;q=0.9" {
		t.Fatalf("BuildProfileHeaders Accept-Language=%q", got)
	}
}

func TestBrowserLocaleForClashNodeUsesTESStandardOffsetNotDST(t *testing.T) {
	june := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	la := browserLocaleForClashNodeAt("🇺🇸 _洛杉矶-18", june)
	if la.TimeZone != -8 {
		t.Fatalf("TES timeZone collector uses January standard offset; Los Angeles should be UTC-8 even in June, got %+v", la)
	}
	denver := browserLocaleForClashNodeAt("_丹佛-01", june)
	if denver.TimeZone != -7 {
		t.Fatalf("TES timeZone collector uses January standard offset; Denver should be UTC-7 even in June, got %+v", denver)
	}
	paris := browserLocaleForClashNodeAt("🇫🇷 _巴黎戴高乐-01", june)
	if paris.TimeZone != 1 {
		t.Fatalf("TES timeZone collector uses January standard offset; Paris should be UTC+1 even in June, got %+v", paris)
	}
}
