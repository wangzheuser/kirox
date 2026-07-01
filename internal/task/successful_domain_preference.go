package task

import (
	"context"
	"fmt"
	"log"
	"strings"

	"reg_go/internal/email"
	"reg_go/internal/storage"
)

const successfulDomainPreferenceMaxCreateAttempts = 10

type tempEmailServiceCreator func() (email.TempEmailService, string, error)

func createTempEmailPreferringSuccessfulDomains(ctx context.Context, provider, providerLabel string, current, total int, create tempEmailServiceCreator) (email.TempEmailService, string, error) {
	preferredDomains := successfulDomainsForProvider(provider)
	if len(preferredDomains) == 0 {
		return create()
	}

	for attempt := 1; attempt <= successfulDomainPreferenceMaxCreateAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		default:
		}

		service, address, err := create()
		if err != nil {
			return nil, "", err
		}

		domain := normalizeEmailAddressDomain(address)
		if _, ok := preferredDomains[domain]; ok {
			if attempt > 1 {
				log.Printf("[Kiro][%d/%d] %s 邮箱命中历史成功域名 %s（第 %d 次创建）", current, total, providerLabel, strings.TrimPrefix(domain, "@"), attempt)
			}
			return service, address, nil
		}

		if attempt < successfulDomainPreferenceMaxCreateAttempts {
			if domain == "" {
				log.Printf("[Kiro][%d/%d] %s 邮箱未能解析域名，重建以优先命中历史成功域名", current, total, providerLabel)
			} else {
				log.Printf("[Kiro][%d/%d] %s 邮箱域名 %s 历史成功率低，重建以优先命中成功域名", current, total, providerLabel, strings.TrimPrefix(domain, "@"))
			}
		}
	}

	return nil, "", fmt.Errorf("%s 连续 %d 次未命中历史成功域名，放弃该临时邮箱并补齐下一次前置创建", providerLabel, successfulDomainPreferenceMaxCreateAttempts)
}

func successfulDomainsForProvider(provider string) map[string]struct{} {
	provider = normalizeSuccessfulDomainPreferenceProvider(provider)
	if provider == "" {
		return nil
	}
	if defaults := defaultSuccessfulDomainsForProvider(provider); len(defaults) > 0 {
		return defaults
	}
	for _, stat := range storage.GetEmailProviderStats() {
		if strings.ToLower(strings.TrimSpace(stat.Provider)) != provider {
			continue
		}
		maxCount := 0
		normalizedCounts := make(map[string]int, len(stat.SuccessDomains))
		for domain, count := range stat.SuccessDomains {
			if count <= 0 {
				continue
			}
			domain = normalizeEmailAddressDomain(domain)
			if domain != "" {
				normalizedCounts[domain] += count
				if normalizedCounts[domain] > maxCount {
					maxCount = normalizedCounts[domain]
				}
			}
		}
		if maxCount <= 0 {
			return nil
		}
		domains := make(map[string]struct{}, len(normalizedCounts))
		for domain, count := range normalizedCounts {
			if count == maxCount {
				domains[domain] = struct{}{}
			}
		}
		return domains
	}
	return nil
}

func defaultSuccessfulDomainsForProvider(provider string) map[string]struct{} {
	switch normalizeSuccessfulDomainPreferenceProvider(provider) {
	case "blinkbox":
		return map[string]struct{}{"@fontdle.com": {}}
	default:
		return nil
	}
}

func normalizeSuccessfulDomainPreferenceProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "smailpro", "blinkbox":
		return strings.ToLower(strings.TrimSpace(provider))
	default:
		return ""
	}
}

func isSuccessfulDomainPreferenceMiss(errorMsg string) bool {
	return strings.Contains(errorMsg, "未命中历史成功域名")
}

func normalizeEmailAddressDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "@") {
		value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
	} else {
		at := strings.LastIndexByte(value, '@')
		if at < 0 || at == len(value)-1 {
			return ""
		}
		value = strings.TrimSpace(value[at+1:])
	}
	if value == "" {
		return ""
	}
	return "@" + value
}
