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

const emailProviderStatsFileName = "email_provider_stats.json"

// EmailProviderStat 是按邮箱渠道持久累计的注册统计。
type EmailProviderStat struct {
	Provider                 string         `json:"provider"`
	OTPReceivedCount         int            `json:"otpReceivedCount"`
	RegistrationSuccessCount int            `json:"registrationSuccessCount"`
	SuccessDomains           map[string]int `json:"successDomains"`
	DomainAttempts           map[string]int `json:"domainAttempts"`
	UpdatedAt                string         `json:"updatedAt"`
}

var emailProviderStatsMu sync.Mutex

func emailProviderStatsFilePath() string {
	return filepath.Join(GetDataDir(), emailProviderStatsFileName)
}

func normalizeStatsProvider(provider string) (string, error) {
	if strings.TrimSpace(provider) == "" {
		return "", fmt.Errorf("邮箱提供商不能为空")
	}
	normalized := normalizeRegistrationEmailProvider(provider)
	if normalized == "" {
		return "", fmt.Errorf("未知邮箱提供商: %s", strings.TrimSpace(provider))
	}
	return normalized, nil
}

func normalizeSuccessDomain(emailAddr string) string {
	emailAddr = strings.ToLower(strings.TrimSpace(emailAddr))
	at := strings.LastIndexByte(emailAddr, '@')
	if at < 0 || at == len(emailAddr)-1 {
		return ""
	}
	domain := strings.TrimSpace(emailAddr[at+1:])
	if domain == "" {
		return ""
	}
	return "@" + domain
}

func readEmailProviderStatsUnlocked() (map[string]EmailProviderStat, error) {
	path := emailProviderStatsFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]EmailProviderStat{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]EmailProviderStat{}, nil
	}
	var stats map[string]EmailProviderStat
	if err := json.Unmarshal(data, &stats); err != nil {
		corruptPath := fmt.Sprintf("%s.corrupt-%s", path, time.Now().Format("20060102-150405.000000000"))
		if renameErr := os.Rename(path, corruptPath); renameErr != nil {
			return nil, fmt.Errorf("解析邮箱渠道统计失败: %w（隔离损坏文件失败: %v）", err, renameErr)
		}
		log.Printf("邮箱渠道统计文件损坏，已隔离为 %s: %v", corruptPath, err)
		return map[string]EmailProviderStat{}, nil
	}
	if stats == nil {
		stats = map[string]EmailProviderStat{}
	}
	for provider, stat := range stats {
		if stat.SuccessDomains == nil {
			stat.SuccessDomains = map[string]int{}
		}
		if stat.DomainAttempts == nil {
			stat.DomainAttempts = map[string]int{}
		}
		if stat.Provider == "" {
			stat.Provider = provider
		}
		stats[provider] = stat
	}
	return stats, nil
}

func writeEmailProviderStatsUnlocked(stats map[string]EmailProviderStat) error {
	path := emailProviderStatsFilePath()
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.WriteFileAtomic(path, data, 0o600)
}

func sortedEmailProviderStats(stats map[string]EmailProviderStat) []EmailProviderStat {
	out := make([]EmailProviderStat, 0, len(stats))
	for provider, stat := range stats {
		if stat.Provider == "" {
			stat.Provider = provider
		}
		if stat.SuccessDomains == nil {
			stat.SuccessDomains = map[string]int{}
		}
		if stat.DomainAttempts == nil {
			stat.DomainAttempts = map[string]int{}
		}
		out = append(out, stat)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Provider < out[j].Provider
	})
	return out
}

// GetEmailProviderStats 返回所有已累计的邮箱渠道统计。
func GetEmailProviderStats() []EmailProviderStat {
	emailProviderStatsMu.Lock()
	defer emailProviderStatsMu.Unlock()

	stats, err := readEmailProviderStatsUnlocked()
	if err != nil {
		log.Printf("读取邮箱渠道统计失败: %v", err)
		return []EmailProviderStat{}
	}
	return sortedEmailProviderStats(stats)
}

// RecordEmailProviderOTPReceived 累计指定渠道的验证码收码成功次数。
func RecordEmailProviderOTPReceived(provider string) error {
	provider, err := normalizeStatsProvider(provider)
	if err != nil {
		return err
	}

	emailProviderStatsMu.Lock()
	defer emailProviderStatsMu.Unlock()

	stats, err := readEmailProviderStatsUnlocked()
	if err != nil {
		return err
	}
	stat := stats[provider]
	stat.Provider = provider
	if stat.SuccessDomains == nil {
		stat.SuccessDomains = map[string]int{}
	}
	if stat.DomainAttempts == nil {
		stat.DomainAttempts = map[string]int{}
	}
	stat.OTPReceivedCount++
	stat.UpdatedAt = time.Now().Format(time.RFC3339)
	stats[provider] = stat
	return writeEmailProviderStatsUnlocked(stats)
}

// RecordEmailProviderRegistrationSuccess 累计指定渠道注册成功数，并按邮箱域名累计成功次数。
func RecordEmailProviderRegistrationSuccess(provider, emailAddr string) error {
	provider, err := normalizeStatsProvider(provider)
	if err != nil {
		return err
	}

	emailProviderStatsMu.Lock()
	defer emailProviderStatsMu.Unlock()

	stats, err := readEmailProviderStatsUnlocked()
	if err != nil {
		return err
	}
	stat := stats[provider]
	stat.Provider = provider
	if stat.SuccessDomains == nil {
		stat.SuccessDomains = map[string]int{}
	}
	if stat.DomainAttempts == nil {
		stat.DomainAttempts = map[string]int{}
	}
	stat.RegistrationSuccessCount++
	if domain := normalizeSuccessDomain(emailAddr); domain != "" {
		stat.SuccessDomains[domain]++
	}
	stat.UpdatedAt = time.Now().Format(time.RFC3339)
	stats[provider] = stat
	return writeEmailProviderStatsUnlocked(stats)
}

// RecordEmailProviderDomainAttempt 累计指定渠道真正进入注册流程的邮箱域名尝试次数。
func RecordEmailProviderDomainAttempt(provider, emailAddr string) error {
	provider, err := normalizeStatsProvider(provider)
	if err != nil {
		return err
	}

	domain := normalizeSuccessDomain(emailAddr)
	if domain == "" {
		return nil
	}

	emailProviderStatsMu.Lock()
	defer emailProviderStatsMu.Unlock()

	stats, err := readEmailProviderStatsUnlocked()
	if err != nil {
		return err
	}
	stat := stats[provider]
	stat.Provider = provider
	if stat.SuccessDomains == nil {
		stat.SuccessDomains = map[string]int{}
	}
	if stat.DomainAttempts == nil {
		stat.DomainAttempts = map[string]int{}
	}
	stat.DomainAttempts[domain]++
	stat.UpdatedAt = time.Now().Format(time.RFC3339)
	stats[provider] = stat
	return writeEmailProviderStatsUnlocked(stats)
}

// ResetEmailProviderStats 清空所有邮箱渠道累计统计。
func ResetEmailProviderStats() error {
	emailProviderStatsMu.Lock()
	defer emailProviderStatsMu.Unlock()

	path := emailProviderStatsFilePath()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
