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
	clashFingerprintPrefix       = "clash:"
	normalClashFingerprintPrefix = "normal-clash:"
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
	if proxy.HasURLTemplate(normalProxyURL) {
		return false
	}
	if !isLoopbackProxyURL(normalProxyURL) {
		return false
	}
	if strings.TrimSpace(clashProxyURL) != "" && !isLoopbackProxyURL(clashProxyURL) {
		return false
	}
	return true
}

func applyClashSelectionToConfig(cfg *core.Config, proxyURL string, selection proxy.ClashSelection, fingerprintPrefix string) core.BrowserLocale {
	return applyClashSelectionToConfigForSubject(cfg, proxyURL, selection, fingerprintPrefix, "")
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

func clashFingerprintKey(prefix, node, subject string) string {
	key := prefix + strings.TrimSpace(node)
	subject = strings.ToLower(strings.TrimSpace(subject))
	if subject == "" {
		return key
	}
	sum := sha256.Sum256([]byte(subject))
	return key + ":acct:" + hex.EncodeToString(sum[:])[:12]
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

func isLoopbackProxyURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		host := u.Hostname()
		return isLoopbackHost(host)
	}
	hostPort := raw
	if i := strings.LastIndexByte(hostPort, '@'); i >= 0 {
		hostPort = hostPort[i+1:]
	}
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	return isLoopbackHost(strings.Trim(host, "[]"))
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
