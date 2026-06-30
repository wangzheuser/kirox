package task

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"strings"

	"reg_go/internal/core"
	"reg_go/internal/proxy"
	"reg_go/internal/storage"
)

const (
	clashFingerprintPrefix          = "clash:"
	normalClashFingerprintPrefix    = "normal-clash:"
	normalTemplateFingerprintPrefix = "normal-template:"
)

func shouldEnableNormalClashAssist(proxyMode, normalProxyURL, clashProxyURL string, clashConfig proxy.ClashConfig, concurrency int) bool {
	if proxyMode != storage.ProxyModeNormal {
		return false
	}
	if !clashConfig.Enabled {
		return false
	}
	if concurrency != 1 {
		return false
	}
	if !sameLoopbackProxyEndpoint(normalProxyURL, clashProxyURL) {
		return false
	}
	return true
}

func applyClashSelectionToConfigForSubject(cfg *core.Config, proxyURL string, selection proxy.ClashSelection, fingerprintPrefix string, subject string) core.BrowserLocale {
	cfg.Proxy = proxyURL
	cfg.FingerprintKey = clashFingerprintKey(fingerprintPrefix, selection.Node, subject)
	locale := core.BrowserLocaleForClashNode(selection.Node)
	cfg.AcceptLanguage = locale.AcceptLanguage
	cfg.I18Next = locale.I18Next
	cfg.TimeZone = locale.TimeZone
	cfg.TimeZoneSet = true
	cfg.ProxySwitchable = true
	return locale
}

func applyRuntimeProxyCountryLocaleToConfigForSubject(cfg *core.Config, countryCode string, subject string) (core.BrowserLocale, bool) {
	locale, ok := runtimeProxyBrowserLocaleForCountryCode(countryCode)
	if !ok {
		return core.BrowserLocale{}, false
	}
	if cfg != nil {
		cfg.AcceptLanguage = locale.AcceptLanguage
		cfg.I18Next = locale.I18Next
		cfg.TimeZone = locale.TimeZone
		cfg.TimeZoneSet = true
	}
	return locale, true
}

func runtimeProxyBrowserLocaleForCountryCode(countryCode string) (core.BrowserLocale, bool) {
	switch strings.ToUpper(strings.TrimSpace(countryCode)) {
	case "US":
		return core.BrowserLocale{AcceptLanguage: "en-US,en;q=0.9", I18Next: "en-US", TimeZone: -8}, true
	case "JP":
		return core.BrowserLocale{AcceptLanguage: "ja-JP,ja;q=0.9,en;q=0.8", I18Next: "ja-JP", TimeZone: 9}, true
	case "DE":
		return core.BrowserLocale{AcceptLanguage: "de-DE,de;q=0.9,en;q=0.8", I18Next: "de-DE", TimeZone: 1}, true
	case "HK":
		return core.BrowserLocale{AcceptLanguage: "zh-HK,zh;q=0.9,en;q=0.8", I18Next: "zh-HK", TimeZone: 8}, true
	case "SG":
		return core.BrowserLocale{AcceptLanguage: "en-SG,en;q=0.9", I18Next: "en-SG", TimeZone: 8}, true
	case "KR":
		return core.BrowserLocale{AcceptLanguage: "ko-KR,ko;q=0.9,en;q=0.8", I18Next: "ko-KR", TimeZone: 9}, true
	case "RO":
		return core.BrowserLocale{AcceptLanguage: "ro-RO,ro;q=0.9,en;q=0.8", I18Next: "ro-RO", TimeZone: 2}, true
	case "GB":
		return core.BrowserLocale{AcceptLanguage: "en-GB,en;q=0.9", I18Next: "en-GB", TimeZone: 0}, true
	case "FR":
		return core.BrowserLocale{AcceptLanguage: "fr-FR,fr;q=0.9,en;q=0.8", I18Next: "fr-FR", TimeZone: 1}, true
	case "CA":
		return core.BrowserLocale{AcceptLanguage: "en-CA,en;q=0.9", I18Next: "en-CA", TimeZone: -5}, true
	case "NL":
		return core.BrowserLocale{AcceptLanguage: "nl-NL,nl;q=0.9,en;q=0.8", I18Next: "nl-NL", TimeZone: 1}, true
	default:
		return core.BrowserLocale{}, false
	}
}

func clashFingerprintKey(prefix, node, subject string) string {
	key := prefix + strings.TrimSpace(node)
	subject = strings.ToLower(strings.TrimSpace(subject))
	if subject == "" {
		return key
	}
	sum := sha256.Sum256([]byte(subject))
	return key + ":acct:" + hex.EncodeToString(sum[:])[:12]
}

func normalTemplateFingerprintKey(proxyURL, subject string) string {
	proxySum := sha256.Sum256([]byte(strings.TrimSpace(proxyURL)))
	key := normalTemplateFingerprintPrefix + hex.EncodeToString(proxySum[:])[:12]
	subject = strings.ToLower(strings.TrimSpace(subject))
	if subject == "" {
		return key
	}
	subjectSum := sha256.Sum256([]byte(subject))
	return key + ":acct:" + hex.EncodeToString(subjectSum[:])[:12]
}

func fingerprintSubjectForTask(cfg *core.Config, fallbackEmail string) string {
	if cfg != nil && cfg.OutlookAccount != nil {
		if email := strings.TrimSpace(cfg.OutlookAccount.RegistrationEmail); email != "" {
			return email
		}
		if email := strings.TrimSpace(cfg.OutlookAccount.Email); email != "" {
			return email
		}
	}
	return strings.TrimSpace(fallbackEmail)
}

func sameLoopbackProxyEndpoint(a, b string) bool {
	aHost, aPort, aOK := loopbackProxyEndpoint(a)
	bHost, bPort, bOK := loopbackProxyEndpoint(b)
	if !aOK || !bOK {
		return false
	}
	if aPort == "" || bPort == "" || aPort != bPort {
		return false
	}
	return isLoopbackHost(aHost) && isLoopbackHost(bHost)
}

func loopbackProxyEndpoint(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	parseable := strings.ReplaceAll(raw, "{uuid}", "00000000-0000-0000-0000-000000000000")
	u, err := url.Parse(parseable)
	if err == nil && u.Host != "" {
		host := u.Hostname()
		if !isLoopbackHost(host) {
			return "", "", false
		}
		return host, u.Port(), true
	}
	hostPort := raw
	if i := strings.LastIndexByte(hostPort, '@'); i >= 0 {
		hostPort = hostPort[i+1:]
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	host = strings.Trim(host, "[]")
	if !isLoopbackHost(host) {
		return "", "", false
	}
	_, port, _ := net.SplitHostPort(hostPort)
	return host, port, true
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
