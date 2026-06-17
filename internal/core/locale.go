package core

import (
	"strings"
	"time"
)

type BrowserLocale struct {
	AcceptLanguage string
	I18Next        string
	TimeZone       int
}

func DefaultBrowserLocale() BrowserLocale {
	return BrowserLocale{AcceptLanguage: "zh-CN,zh;q=0.9,en;q=0.8", I18Next: "zh-CN", TimeZone: 8}
}

func BrowserLocaleForClashNode(node string) BrowserLocale {
	return browserLocaleForClashNodeAt(node, time.Now())
}

func browserLocaleForClashNodeAt(node string, at time.Time) BrowserLocale {
	name := strings.ToLower(strings.TrimSpace(node))
	switch {
	case strings.Contains(name, "丹佛") || strings.Contains(name, "denver"):
		return BrowserLocale{AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: tesStandardTimezoneOffsetHours("America/Denver", at, -7)}
	case strings.Contains(name, "🇺🇸") || strings.Contains(name, "美国") || strings.Contains(name, "洛杉矶") || strings.Contains(name, "圣何塞"):
		return BrowserLocale{AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: tesStandardTimezoneOffsetHours("America/Los_Angeles", at, -8)}
	case strings.Contains(name, "🇯🇵") || strings.Contains(name, "日本") || strings.Contains(name, "东京") || strings.Contains(name, "大阪"):
		return BrowserLocale{AcceptLanguage: "ja-JP,ja;q=0.9,en;q=0.8", I18Next: "ja-JP", TimeZone: 9}
	case strings.Contains(name, "🇫🇷") || strings.Contains(name, "法国") || strings.Contains(name, "巴黎"):
		return BrowserLocale{AcceptLanguage: "fr-FR,fr;q=0.9,en;q=0.8", I18Next: "fr-FR", TimeZone: tesStandardTimezoneOffsetHours("Europe/Paris", at, 1)}
	case strings.Contains(name, "🇭🇰") || strings.Contains(name, "香港"):
		return BrowserLocale{AcceptLanguage: "zh-HK,zh;q=0.9,en;q=0.8", I18Next: "zh-HK", TimeZone: 8}
	case strings.Contains(name, "🇸🇬") || strings.Contains(name, "新加坡"):
		return BrowserLocale{AcceptLanguage: "en-SG,en;q=0.9", I18Next: "en-SG", TimeZone: 8}
	case strings.Contains(name, "🇻🇳") || strings.Contains(name, "越南") || strings.Contains(name, "河内"):
		return BrowserLocale{AcceptLanguage: "vi-VN,vi;q=0.9,en;q=0.8", I18Next: "vi-VN", TimeZone: 7}
	case strings.Contains(name, "🇲🇾") || strings.Contains(name, "马来西亚") || strings.Contains(name, "吉隆坡"):
		return BrowserLocale{AcceptLanguage: "en-MY,en;q=0.9", I18Next: "en-MY", TimeZone: 8}
	case strings.Contains(name, "🇮🇩") || strings.Contains(name, "印度尼西亚"):
		return BrowserLocale{AcceptLanguage: "id-ID,id;q=0.9,en;q=0.8", I18Next: "id-ID", TimeZone: 7}
	default:
		return DefaultBrowserLocale()
	}
}

func tesStandardTimezoneOffsetHours(name string, at time.Time, fallback int) int {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return fallback
	}
	// AWS TES 的 timeZone collector 使用当年 1 月 10 日的 GMT 偏移，
	// 等价于浏览器标准时区偏移而不是当前夏令时偏移。
	standard := time.Date(at.In(loc).Year(), time.January, 10, 0, 0, 0, 0, loc)
	_, offset := standard.Zone()
	return offset / 3600
}

func (c *Config) BrowserLocale() BrowserLocale {
	if c == nil || strings.TrimSpace(c.AcceptLanguage) == "" || strings.TrimSpace(c.I18Next) == "" {
		loc := DefaultBrowserLocale()
		if c != nil && strings.TrimSpace(c.AcceptLanguage) != "" {
			loc.AcceptLanguage = strings.TrimSpace(c.AcceptLanguage)
		}
		if c != nil && strings.TrimSpace(c.I18Next) != "" {
			loc.I18Next = strings.TrimSpace(c.I18Next)
		}
		if c != nil && c.TimeZoneSet {
			loc.TimeZone = c.TimeZone
		}
		return loc
	}
	loc := BrowserLocale{AcceptLanguage: strings.TrimSpace(c.AcceptLanguage), I18Next: strings.TrimSpace(c.I18Next), TimeZone: c.TimeZone}
	if !c.TimeZoneSet {
		loc.TimeZone = DefaultBrowserLocale().TimeZone
	}
	return loc
}
