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
	case strings.Contains(name, "阿什本") || strings.Contains(name, "纽约") || strings.Contains(name, "new york") || strings.Contains(name, "ashburn"):
		return BrowserLocale{AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: tesStandardTimezoneOffsetHours("America/New_York", at, -5)}
	case strings.Contains(name, "芝加哥") || strings.Contains(name, "达拉斯") || strings.Contains(name, "chicago") || strings.Contains(name, "dallas"):
		return BrowserLocale{AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: tesStandardTimezoneOffsetHours("America/Chicago", at, -6)}
	case strings.Contains(name, "犹他") || strings.Contains(name, "utah"):
		return BrowserLocale{AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: tesStandardTimezoneOffsetHours("America/Denver", at, -7)}
	case strings.Contains(name, "🇺🇸") || strings.Contains(name, "美国") || strings.Contains(name, "洛杉矶") || strings.Contains(name, "圣何塞"):
		return BrowserLocale{AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: tesStandardTimezoneOffsetHours("America/Los_Angeles", at, -8)}
	case strings.Contains(name, "🇯🇵") || strings.Contains(name, "日本") || strings.Contains(name, "东京") || strings.Contains(name, "大阪"):
		return BrowserLocale{AcceptLanguage: "ja-JP,ja;q=0.9,en;q=0.8", I18Next: "ja-JP", TimeZone: 9}
	case strings.Contains(name, "🇫🇷") || strings.Contains(name, "法国") || strings.Contains(name, "巴黎"):
		return BrowserLocale{AcceptLanguage: "fr-FR,fr;q=0.9,en;q=0.8", I18Next: "fr-FR", TimeZone: tesStandardTimezoneOffsetHours("Europe/Paris", at, 1)}
	case strings.Contains(name, "英国") || strings.Contains(name, "伦敦") || strings.Contains(name, "🇬🇧") || strings.Contains(name, "london"):
		return BrowserLocale{AcceptLanguage: "en-GB,en;q=0.9", I18Next: "en-GB", TimeZone: tesStandardTimezoneOffsetHours("Europe/London", at, 0)}
	case strings.Contains(name, "爱尔兰") || strings.Contains(name, "都柏林") || strings.Contains(name, "ireland") || strings.Contains(name, "dublin"):
		return BrowserLocale{AcceptLanguage: "en-IE,en;q=0.9", I18Next: "en-IE", TimeZone: tesStandardTimezoneOffsetHours("Europe/Dublin", at, 0)}
	case strings.Contains(name, "德国") || strings.Contains(name, "纽伦堡") || strings.Contains(name, "germany"):
		return BrowserLocale{AcceptLanguage: "de-DE,de;q=0.9,en;q=0.8", I18Next: "de-DE", TimeZone: tesStandardTimezoneOffsetHours("Europe/Berlin", at, 1)}
	case strings.Contains(name, "意大利") || strings.Contains(name, "italy"):
		return BrowserLocale{AcceptLanguage: "it-IT,it;q=0.9,en;q=0.8", I18Next: "it-IT", TimeZone: tesStandardTimezoneOffsetHours("Europe/Rome", at, 1)}
	case strings.Contains(name, "荷兰") || strings.Contains(name, "netherlands"):
		return BrowserLocale{AcceptLanguage: "nl-NL,nl;q=0.9,en;q=0.8", I18Next: "nl-NL", TimeZone: tesStandardTimezoneOffsetHours("Europe/Amsterdam", at, 1)}
	case strings.Contains(name, "瑞士") || strings.Contains(name, "苏黎世") || strings.Contains(name, "switzerland"):
		return BrowserLocale{AcceptLanguage: "de-CH,de;q=0.9,en;q=0.8", I18Next: "de-CH", TimeZone: tesStandardTimezoneOffsetHours("Europe/Zurich", at, 1)}
	case strings.Contains(name, "西班牙") || strings.Contains(name, "spain"):
		return BrowserLocale{AcceptLanguage: "es-ES,es;q=0.9,en;q=0.8", I18Next: "es-ES", TimeZone: tesStandardTimezoneOffsetHours("Europe/Madrid", at, 1)}
	case strings.Contains(name, "罗马尼亚") || strings.Contains(name, "romania"):
		return BrowserLocale{AcceptLanguage: "ro-RO,ro;q=0.9,en;q=0.8", I18Next: "ro-RO", TimeZone: tesStandardTimezoneOffsetHours("Europe/Bucharest", at, 2)}
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
	case strings.Contains(name, "印度") || strings.Contains(name, "海得拉巴"):
		return BrowserLocale{AcceptLanguage: "en-IN,en;q=0.9", I18Next: "en-IN", TimeZone: 5}
	case strings.Contains(name, "泰国"):
		return BrowserLocale{AcceptLanguage: "th-TH,th;q=0.9,en;q=0.8", I18Next: "th-TH", TimeZone: 7}
	case strings.Contains(name, "澳大利亚") || strings.Contains(name, "墨尔本") || strings.Contains(name, "australia"):
		return BrowserLocale{AcceptLanguage: "en-AU,en;q=0.9", I18Next: "en-AU", TimeZone: tesStandardTimezoneOffsetHours("Australia/Melbourne", at, 10)}
	case strings.Contains(name, "沙特") || strings.Contains(name, "吉达") || strings.Contains(name, "saudi"):
		return BrowserLocale{AcceptLanguage: "ar-SA,ar;q=0.9,en;q=0.8", I18Next: "ar-SA", TimeZone: 3}
	case strings.Contains(name, "尼日利亚") || strings.Contains(name, "nigeria"):
		return BrowserLocale{AcceptLanguage: "en-NG,en;q=0.9", I18Next: "en-NG", TimeZone: 1}
	case strings.Contains(name, "土耳其") || strings.Contains(name, "turkey"):
		return BrowserLocale{AcceptLanguage: "tr-TR,tr;q=0.9,en;q=0.8", I18Next: "tr-TR", TimeZone: 3}
	case strings.Contains(name, "俄罗斯") || strings.Contains(name, "圣彼得堡") || strings.Contains(name, "russia"):
		return BrowserLocale{AcceptLanguage: "ru-RU,ru;q=0.9,en;q=0.8", I18Next: "ru-RU", TimeZone: 3}
	case strings.Contains(name, "加拿大") || strings.Contains(name, "多伦多") || strings.Contains(name, "canada") || strings.Contains(name, "toronto"):
		return BrowserLocale{AcceptLanguage: "en-CA,en;q=0.9", I18Next: "en-CA", TimeZone: tesStandardTimezoneOffsetHours("America/Toronto", at, -5)}
	case strings.Contains(name, "以色列") || strings.Contains(name, "israel"):
		return BrowserLocale{AcceptLanguage: "he-IL,he;q=0.9,en;q=0.8", I18Next: "he-IL", TimeZone: tesStandardTimezoneOffsetHours("Asia/Jerusalem", at, 2)}
	case strings.Contains(name, "墨西哥") || strings.Contains(name, "蒙特雷") || strings.Contains(name, "mexico"):
		return BrowserLocale{AcceptLanguage: "es-MX,es;q=0.9,en;q=0.8", I18Next: "es-MX", TimeZone: tesStandardTimezoneOffsetHours("America/Monterrey", at, -6)}
	case strings.Contains(name, "巴西") || strings.Contains(name, "圣保罗") || strings.Contains(name, "维涅社") || strings.Contains(name, "brazil"):
		return BrowserLocale{AcceptLanguage: "pt-BR,pt;q=0.9,en;q=0.8", I18Next: "pt-BR", TimeZone: tesStandardTimezoneOffsetHours("America/Sao_Paulo", at, -3)}
	case strings.Contains(name, "南非") || strings.Contains(name, "south africa"):
		return BrowserLocale{AcceptLanguage: "en-ZA,en;q=0.9", I18Next: "en-ZA", TimeZone: 2}
	case strings.Contains(name, "哥伦比亚") || strings.Contains(name, "colombia"):
		return BrowserLocale{AcceptLanguage: "es-CO,es;q=0.9,en;q=0.8", I18Next: "es-CO", TimeZone: -5}
	case strings.Contains(name, "智利") || strings.Contains(name, "chile"):
		return BrowserLocale{AcceptLanguage: "es-CL,es;q=0.9,en;q=0.8", I18Next: "es-CL", TimeZone: tesStandardTimezoneOffsetHours("America/Santiago", at, -4)}
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
