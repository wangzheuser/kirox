package data

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var accountsJSONMu sync.RWMutex

// SaveKiroSuccess 以明文 JSON 数组形式把成功注册的账号写入 outDir/accounts.json。
// 同邮箱以最新一条覆盖；仅处理成功记录（失败/封号不落盘，只留在运行日志）。
func SaveKiroSuccess(result map[string]interface{}, outDir string) error {
	if result == nil || result["status"] != "success" {
		return nil
	}
	emailAddr, _ := result["email"].(string)
	if emailAddr == "" {
		return fmt.Errorf("缺少 email 字段")
	}

	at, _ := result["aws_token"].(map[string]interface{})
	if at == nil {
		at = map[string]interface{}{}
	}
	verify, _ := result["verify"].(map[string]interface{})
	now := time.Now().Format("2006-01-02 15:04:05")
	item := map[string]interface{}{
		"password":     result["password"],
		"accessToken":  at["accessToken"],
		"refreshToken": at["refreshToken"],
		"kiroRsSynced": false,
		"provider":     "BuilderId",
		"clientId":     result["client_id"],
		"clientSecret": result["client_secret"],
		"region":       "us-east-1",
		"email":        emailAddr,
		"time":         now,
		"priority":     calculateAccountPriority(now),
	}
	if verify != nil {
		item["creditUsed"] = verify["credit_used"]
		item["creditLimit"] = verify["credit_limit"]
		item["subscription"] = verify["subscription"]
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败: %w", err)
	}
	path := filepath.Join(outDir, "accounts.json")

	accountsJSONMu.Lock()
	defer accountsJSONMu.Unlock()

	existing, err := loadJSONArray(path)
	if err != nil {
		return fmt.Errorf("读取 accounts.json 失败: %w", err)
	}

	merged := make([]map[string]interface{}, 0, len(existing)+1)
	for _, e := range existing {
		if em, _ := e["email"].(string); em == emailAddr {
			oldRefresh, _ := e["refreshToken"].(string)
			newRefresh, _ := item["refreshToken"].(string)
			if oldRefresh == newRefresh {
				if synced, _ := e["kiroRsSynced"].(bool); synced {
					item["kiroRsSynced"] = true
				}
			}
			continue
		}
		merged = append(merged, e)
	}
	merged = append(merged, item)

	if err := writeJSONArrayAtomic(path, merged); err != nil {
		return fmt.Errorf("写入 accounts.json 失败: %w", err)
	}
	log.Printf("[Kiro] 结果已保存: %s", path)
	return nil
}

// LoadAccounts 读取 outDir/accounts.json 中保存的账号列表（按写入顺序返回）。
func LoadAccounts(outDir string) ([]map[string]interface{}, error) {
	accountsJSONMu.RLock()
	defer accountsJSONMu.RUnlock()

	return loadJSONArray(filepath.Join(outDir, "accounts.json"))
}

// MarkKiroRSSynced 将指定邮箱的账号标记为已同步到 kiro.rs。
func MarkKiroRSSynced(outDir string, emails []string) (int, error) {
	if len(emails) == 0 {
		return 0, nil
	}
	wanted := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			wanted[email] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return 0, nil
	}

	path := filepath.Join(outDir, "accounts.json")
	accountsJSONMu.Lock()
	defer accountsJSONMu.Unlock()

	existing, err := loadJSONArray(path)
	if err != nil || len(existing) == 0 {
		return 0, err
	}

	updated := 0
	for _, item := range existing {
		email, _ := item["email"].(string)
		if _, ok := wanted[strings.ToLower(strings.TrimSpace(email))]; !ok {
			continue
		}
		if synced, _ := item["kiroRsSynced"].(bool); !synced {
			updated++
		}
		item["kiroRsSynced"] = true
	}
	if updated == 0 {
		return 0, nil
	}
	if err := writeJSONArrayAtomic(path, existing); err != nil {
		return 0, err
	}
	return updated, nil
}

// DeleteAccount 从 outDir/accounts.json 中移除指定邮箱的账号；返回是否实际删除。
func DeleteAccount(outDir, email string) (bool, error) {
	path := filepath.Join(outDir, "accounts.json")
	accountsJSONMu.Lock()
	defer accountsJSONMu.Unlock()

	existing, err := loadJSONArray(path)
	if err != nil || len(existing) == 0 {
		return false, err
	}
	out := make([]map[string]interface{}, 0, len(existing))
	removed := false
	for _, e := range existing {
		if em, _ := e["email"].(string); em == email {
			removed = true
			continue
		}
		out = append(out, e)
	}
	if !removed {
		return false, nil
	}
	if err := writeJSONArrayAtomic(path, out); err != nil {
		return false, err
	}
	return true, nil
}

func loadJSONArray(path string) ([]map[string]interface{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, err
	}
	return arr, nil
}

func writeJSONArrayAtomic(path string, arr []map[string]interface{}) error {
	b, err := json.MarshalIndent(arr, "", "  ")
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	if _, err := tmpFile.Write(b); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(0o644); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
