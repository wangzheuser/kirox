package core

import (
	"strings"
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

func TestBrowserLocaleForCurrentClashRegions(t *testing.T) {
	tests := []struct {
		name       string
		node       string
		acceptHead string
		i18next    string
		timeZone   int
	}{
		{name: "英国伦敦", node: "英国伦敦1-50Gbps-高性能-AN | 0x", acceptHead: "en-GB", i18next: "en-GB", timeZone: 0},
		{name: "澳大利亚墨尔本", node: "澳大利亚墨尔本1-AN | 1x", acceptHead: "en-AU", i18next: "en-AU", timeZone: 11},
		{name: "加拿大多伦多", node: "加拿大多伦多1-AN | 1x", acceptHead: "en-CA", i18next: "en-CA", timeZone: -5},
		{name: "印度海得拉巴", node: "印度-海得拉巴1-AN | 1x", acceptHead: "en-IN", i18next: "en-IN", timeZone: 5},
		{name: "泰国", node: "泰国1-1Gbps-高性能-AN | 1x", acceptHead: "th-TH", i18next: "th-TH", timeZone: 7},
		{name: "德国", node: "德国纽伦堡1-1Gbps-高性能-AN | 0x", acceptHead: "de-DE", i18next: "de-DE", timeZone: 1},
		{name: "巴西", node: "巴西圣保罗1-AN | 1x", acceptHead: "pt-BR", i18next: "pt-BR", timeZone: -3},
		{name: "南非", node: "南非1-AN | 1x", acceptHead: "en-ZA", i18next: "en-ZA", timeZone: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := browserLocaleForClashNodeAt(tt.node, time.Date(2026, time.June, 20, 12, 0, 0, 0, time.UTC))
			if !strings.HasPrefix(loc.AcceptLanguage, tt.acceptHead) || loc.I18Next != tt.i18next || loc.TimeZone != tt.timeZone {
				t.Fatalf("locale mismatch for %s: got %+v", tt.node, loc)
			}
		})
	}
}
