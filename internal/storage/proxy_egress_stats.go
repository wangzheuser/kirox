package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"reg_go/internal/fileutil"
)

const proxyEgressStatsFileName = "proxy_egress_stats.json"

// ProxyEgressIdentity 描述一次代理出口探测得到的稳定身份。
type ProxyEgressIdentity struct {
	IP          string
	CountryCode string
	ISP         string
	ASN         string
}

// ProxyEgressStat 是按代理来源和出口 IP 持久累计的注册统计。
type ProxyEgressStat struct {
	SourceKey           string `json:"sourceKey"`
	IP                  string `json:"ip"`
	CountryCode         string `json:"countryCode"`
	ISP                 string `json:"isp"`
	ASN                 string `json:"asn"`
	AttemptCount        int    `json:"attemptCount"`
	SuccessCount        int    `json:"successCount"`
	RiskFailureCount    int    `json:"riskFailureCount"`
	NetworkFailureCount int    `json:"networkFailureCount"`
	CooldownUntil       string `json:"cooldownUntil,omitempty"`
	UpdatedAt           string `json:"updatedAt"`
}

var proxyEgressStatsMu sync.Mutex

func proxyEgressStatsFilePath() string {
	return filepath.Join(GetDataDir(), proxyEgressStatsFileName)
}

func normalizeProxyEgressSourceKey(sourceKey string) (string, error) {
	sourceKey = strings.TrimSpace(sourceKey)
	if sourceKey == "" {
		return "", fmt.Errorf("代理出口来源不能为空")
	}
	return sourceKey, nil
}

func normalizeProxyEgressIdentity(identity ProxyEgressIdentity) (ProxyEgressIdentity, error) {
	identity.IP = strings.TrimSpace(identity.IP)
	identity.CountryCode = strings.ToUpper(strings.TrimSpace(identity.CountryCode))
	identity.ISP = strings.TrimSpace(identity.ISP)
	identity.ASN = strings.TrimSpace(identity.ASN)
	if identity.IP == "" {
		return ProxyEgressIdentity{}, fmt.Errorf("代理出口 IP 不能为空")
	}
	return identity, nil
}

func proxyEgressStatKey(sourceKey, ip string) string {
	return sourceKey + "\x00" + ip
}

func readProxyEgressStatsUnlocked() (map[string]ProxyEgressStat, error) {
	path := proxyEgressStatsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]ProxyEgressStat{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]ProxyEgressStat{}, nil
	}
	var stats map[string]ProxyEgressStat
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}
	if stats == nil {
		stats = map[string]ProxyEgressStat{}
	}
	normalized := make(map[string]ProxyEgressStat, len(stats))
	for key, stat := range stats {
		sourceKey := strings.TrimSpace(stat.SourceKey)
		ip := strings.TrimSpace(stat.IP)
		if sourceKey == "" || ip == "" {
			parts := strings.SplitN(key, "\x00", 2)
			if len(parts) == 2 {
				if sourceKey == "" {
					sourceKey = strings.TrimSpace(parts[0])
				}
				if ip == "" {
					ip = strings.TrimSpace(parts[1])
				}
			}
		}
		if sourceKey == "" || ip == "" {
			continue
		}
		stat.SourceKey = sourceKey
		stat.IP = ip
		stat.CountryCode = strings.ToUpper(strings.TrimSpace(stat.CountryCode))
		stat.ISP = strings.TrimSpace(stat.ISP)
		stat.ASN = strings.TrimSpace(stat.ASN)
		normalized[proxyEgressStatKey(sourceKey, ip)] = stat
	}
	return normalized, nil
}

func writeProxyEgressStatsUnlocked(stats map[string]ProxyEgressStat) error {
	path := proxyEgressStatsFilePath()
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func sortedProxyEgressStats(stats map[string]ProxyEgressStat) []ProxyEgressStat {
	out := make([]ProxyEgressStat, 0, len(stats))
	for _, stat := range stats {
		out = append(out, stat)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceKey == out[j].SourceKey {
			return out[i].IP < out[j].IP
		}
		return out[i].SourceKey < out[j].SourceKey
	})
	return out
}

// GetProxyEgressStats 返回所有已累计的代理出口统计。
func GetProxyEgressStats() []ProxyEgressStat {
	proxyEgressStatsMu.Lock()
	defer proxyEgressStatsMu.Unlock()

	stats, err := readProxyEgressStatsUnlocked()
	if err != nil {
		log.Printf("读取代理出口统计失败: %v", err)
		return []ProxyEgressStat{}
	}
	return sortedProxyEgressStats(stats)
}

func modifyProxyEgressStat(sourceKey string, identity ProxyEgressIdentity, fn func(*ProxyEgressStat)) error {
	sourceKey, err := normalizeProxyEgressSourceKey(sourceKey)
	if err != nil {
		return err
	}
	identity, err = normalizeProxyEgressIdentity(identity)
	if err != nil {
		return err
	}

	proxyEgressStatsMu.Lock()
	defer proxyEgressStatsMu.Unlock()

	stats, err := readProxyEgressStatsUnlocked()
	if err != nil {
		return err
	}
	key := proxyEgressStatKey(sourceKey, identity.IP)
	stat := stats[key]
	stat.SourceKey = sourceKey
	stat.IP = identity.IP
	if identity.CountryCode != "" {
		stat.CountryCode = identity.CountryCode
	}
	if identity.ISP != "" {
		stat.ISP = identity.ISP
	}
	if identity.ASN != "" {
		stat.ASN = identity.ASN
	}
	fn(&stat)
	stat.UpdatedAt = time.Now().Format(time.RFC3339)
	stats[key] = stat
	return writeProxyEgressStatsUnlocked(stats)
}

// RecordProxyEgressAttempt 累计指定代理出口真正进入注册流程的尝试次数。
func RecordProxyEgressAttempt(sourceKey string, identity ProxyEgressIdentity) error {
	return modifyProxyEgressStat(sourceKey, identity, func(stat *ProxyEgressStat) {
		stat.AttemptCount++
	})
}

// RecordProxyEgressRegistrationSuccess 累计指定代理出口注册成功次数，并清除冷却。
func RecordProxyEgressRegistrationSuccess(sourceKey string, identity ProxyEgressIdentity) error {
	return modifyProxyEgressStat(sourceKey, identity, func(stat *ProxyEgressStat) {
		stat.SuccessCount++
		stat.CooldownUntil = ""
	})
}

// RecordProxyEgressRiskFailure 累计指定代理出口风控失败，并冷却该出口 IP。
func RecordProxyEgressRiskFailure(sourceKey string, identity ProxyEgressIdentity, cooldown time.Duration) error {
	return modifyProxyEgressStat(sourceKey, identity, func(stat *ProxyEgressStat) {
		stat.RiskFailureCount++
		if cooldown > 0 {
			stat.CooldownUntil = time.Now().Add(cooldown).Format(time.RFC3339)
		}
	})
}

// RecordProxyEgressNetworkFailure 累计指定代理出口网络失败次数。
func RecordProxyEgressNetworkFailure(sourceKey string, identity ProxyEgressIdentity) error {
	return modifyProxyEgressStat(sourceKey, identity, func(stat *ProxyEgressStat) {
		stat.NetworkFailureCount++
	})
}

// IsProxyEgressCooling 判断指定代理来源下的出口 IP 是否仍在风控冷却期。
func IsProxyEgressCooling(sourceKey, ip string, now time.Time) bool {
	sourceKey = strings.TrimSpace(sourceKey)
	ip = strings.TrimSpace(ip)
	if sourceKey == "" || ip == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}

	proxyEgressStatsMu.Lock()
	defer proxyEgressStatsMu.Unlock()

	stats, err := readProxyEgressStatsUnlocked()
	if err != nil {
		log.Printf("读取代理出口统计失败: %v", err)
		return false
	}
	stat, ok := stats[proxyEgressStatKey(sourceKey, ip)]
	if !ok || strings.TrimSpace(stat.CooldownUntil) == "" {
		return false
	}
	until, err := time.Parse(time.RFC3339, stat.CooldownUntil)
	if err != nil {
		return false
	}
	return now.Before(until)
}

// HasProxyEgressSuccess 判断指定代理来源下的出口 IP 是否有历史注册成功记录。
func HasProxyEgressSuccess(sourceKey, ip string) bool {
	sourceKey = strings.TrimSpace(sourceKey)
	ip = strings.TrimSpace(ip)
	if sourceKey == "" || ip == "" {
		return false
	}

	proxyEgressStatsMu.Lock()
	defer proxyEgressStatsMu.Unlock()

	stats, err := readProxyEgressStatsUnlocked()
	if err != nil {
		log.Printf("读取代理出口统计失败: %v", err)
		return false
	}
	stat, ok := stats[proxyEgressStatKey(sourceKey, ip)]
	return ok && stat.SuccessCount > 0
}

// ResetProxyEgressStats 清空所有代理出口累计统计。
func ResetProxyEgressStats() error {
	proxyEgressStatsMu.Lock()
	defer proxyEgressStatsMu.Unlock()

	path := proxyEgressStatsFilePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
