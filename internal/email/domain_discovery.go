package email

import "strings"

func normalizeEmailDomain(domain string) string {
	domain = strings.ToLower(strings.TrimSpace(domain))
	domain = strings.TrimPrefix(domain, "@")
	domain = strings.Trim(domain, `"'<>(),;:[]{} `+"\r\n\t")
	if domain == "" || strings.ContainsAny(domain, "/\\@") || strings.Contains(domain, "..") {
		return ""
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return ""
	}
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return ""
		}
		for _, r := range part {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return ""
		}
	}
	tld := parts[len(parts)-1]
	if len(tld) < 2 {
		return ""
	}
	for _, r := range tld {
		if r < 'a' || r > 'z' {
			return ""
		}
	}
	return domain
}

func appendUniqueDomains(out []string, domains ...string) []string {
	seen := make(map[string]struct{}, len(out)+len(domains))
	for _, domain := range out {
		if normalized := normalizeEmailDomain(domain); normalized != "" {
			seen[normalized] = struct{}{}
		}
	}
	for _, domain := range domains {
		normalized := normalizeEmailDomain(domain)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}
