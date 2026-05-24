package data

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAccountProvider = "BuilderId"
	defaultAccountRegion   = "us-east-1"
)

// AccountPoolImportSummary 描述账号池导入结果。
type AccountPoolImportSummary struct {
	Imported int `json:"imported"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"`
	Total    int `json:"total"`
}

type accountPoolExportRecord struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	Priority     int    `json:"priority"`
}

// ListAccountPool 读取账号池并为前端补齐可展示的 priority。
func ListAccountPool(outDir string) ([]map[string]interface{}, error) {
	items, err := LoadAccounts(outDir)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		ensureAccountPoolDefaults(item, false)
	}
	return items, nil
}

// ImportAccountPoolJSON 导入参考插件导出的账号 JSON 数组。
func ImportAccountPoolJSON(outDir, raw string) (AccountPoolImportSummary, error) {
	var summary AccountPoolImportSummary
	var incoming []map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &incoming); err != nil {
		return summary, fmt.Errorf("账号 JSON 格式无效: %w", err)
	}

	existing, err := LoadAccounts(outDir)
	if err != nil {
		return summary, err
	}
	index := make(map[string]int, len(existing))
	for i, item := range existing {
		if email := strings.ToLower(strings.TrimSpace(stringField(item, "email"))); email != "" {
			index[email] = i
		}
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	for _, src := range incoming {
		email, ok := stringFieldIfPresent(src, "email")
		email = strings.TrimSpace(email)
		if !ok || email == "" {
			summary.Skipped++
			continue
		}

		key := strings.ToLower(email)
		if pos, exists := index[key]; exists {
			mergeAccountPoolFields(existing[pos], src)
			existing[pos]["email"] = email
			ensureAccountPoolDefaults(existing[pos], false)
			summary.Updated++
			continue
		}

		item := map[string]interface{}{
			"email":    email,
			"provider": defaultAccountProvider,
			"region":   defaultAccountRegion,
			"time":     now,
		}
		mergeAccountPoolFields(item, src)
		if provider, ok := stringFieldIfPresent(src, "provider"); ok && strings.TrimSpace(provider) != "" {
			item["provider"] = strings.TrimSpace(provider)
		}
		if region, ok := stringFieldIfPresent(src, "region"); ok && strings.TrimSpace(region) != "" {
			item["region"] = strings.TrimSpace(region)
		}
		if t, ok := stringFieldIfPresent(src, "time"); ok && strings.TrimSpace(t) != "" {
			item["time"] = strings.TrimSpace(t)
		}
		if sub, ok := stringFieldIfPresent(src, "subscription"); ok {
			item["subscription"] = sub
		}
		ensureAccountPoolDefaults(item, true)
		existing = append(existing, item)
		index[key] = len(existing) - 1
		summary.Imported++
	}

	if err := writeJSONArrayAtomic(accountsPath(outDir), existing); err != nil {
		return summary, err
	}
	summary.Total = len(existing)
	return summary, nil
}

// ExportAccountPoolJSON 导出参考插件兼容的账号 JSON 数组字符串。
func ExportAccountPoolJSON(outDir string) (string, int, error) {
	items, err := ListAccountPool(outDir)
	if err != nil {
		return "", 0, err
	}
	records := make([]accountPoolExportRecord, 0, len(items))
	for _, item := range items {
		records = append(records, accountPoolExportRecord{
			Email:        stringField(item, "email"),
			Password:     stringField(item, "password"),
			ClientID:     stringField(item, "clientId"),
			ClientSecret: stringField(item, "clientSecret"),
			AccessToken:  stringField(item, "accessToken"),
			RefreshToken: stringField(item, "refreshToken"),
			Priority:     priorityField(item),
		})
	}
	b, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return "", 0, err
	}
	return string(b), len(records), nil
}

func accountsPath(outDir string) string {
	return filepath.Join(outDir, "accounts.json")
}

func mergeAccountPoolFields(dst, src map[string]interface{}) {
	for _, key := range []string{"password", "clientId", "clientSecret", "accessToken", "refreshToken"} {
		if value, ok := stringFieldIfPresent(src, key); ok {
			dst[key] = value
		}
	}
	if priority, ok := priorityFieldIfPresent(src, "priority"); ok {
		dst["priority"] = priority
	}
}

func ensureAccountPoolDefaults(item map[string]interface{}, forcePriority bool) {
	if item == nil {
		return
	}
	if strings.TrimSpace(stringField(item, "provider")) == "" {
		item["provider"] = defaultAccountProvider
	}
	if strings.TrimSpace(stringField(item, "region")) == "" {
		item["region"] = defaultAccountRegion
	}
	if forcePriority || !hasNonEmptyField(item, "priority") {
		item["priority"] = calculateAccountPriority(stringField(item, "time"))
	}
}

func hasNonEmptyField(item map[string]interface{}, key string) bool {
	v, ok := item[key]
	if !ok || v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) != ""
	}
	return true
}

func stringField(item map[string]interface{}, key string) string {
	value, _ := stringFieldIfPresent(item, key)
	return value
}

func stringFieldIfPresent(item map[string]interface{}, key string) (string, bool) {
	if item == nil {
		return "", false
	}
	value, ok := item[key]
	if !ok || value == nil {
		return "", ok
	}
	switch v := value.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	default:
		return fmt.Sprint(v), true
	}
}

func priorityField(item map[string]interface{}) int {
	if priority, ok := priorityFieldIfPresent(item, "priority"); ok {
		return priority
	}
	return calculateAccountPriority(stringField(item, "time"))
}

func priorityFieldIfPresent(item map[string]interface{}, key string) (int, bool) {
	if item == nil {
		return 0, false
	}
	value, ok := item[key]
	if !ok || value == nil {
		return 0, false
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := strconv.Atoi(v.String())
		return i, err == nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		return i, err == nil
	default:
		return 0, false
	}
}

func calculateAccountPriority(timeStr string) int {
	t, ok := parseAccountTime(timeStr)
	if !ok {
		return 9999
	}
	t = t.AddDate(0, 1, 0)
	return int(t.Month())*100 + t.Day()
}

func parseAccountTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006/1/2 15:04:05",
		"2006/01/02 15:04:05",
		"2006-01-02",
		"2006/1/2",
		"2006/01/02",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
