package email

import (
	"os"
	"strings"
	"time"

	"reg_go/internal/storage"
)

// ParseOutlook 解析 Outlook 账号
func ParseOutlook(data string) map[string]interface{} {
	accounts := ParseOutlookLines(data)

	var accountList []map[string]string
	for _, acc := range accounts {
		accountList = append(accountList, map[string]string{
			"email":    acc.Email,
			"password": acc.Password,
		})
	}

	return map[string]interface{}{
		"count":    len(accounts),
		"accounts": accountList,
	}
}

// AddOutlookAccounts 添加 Outlook 账号到持久化存储
func AddOutlookAccounts(data string) map[string]interface{} {
	accounts := ParseOutlookLines(data)
	if len(accounts) == 0 {
		return map[string]interface{}{"error": "未解析到有效账号"}
	}

	addedCount := 0
	now := time.Now().Format("2006-01-02 15:04:05")
	storage.ModifyAccountsCached(func(existing []map[string]interface{}) []map[string]interface{} {
		for _, acc := range accounts {
			exists := false
			for _, e := range existing {
				if e["email"] == acc.Email {
					exists = true
					break
				}
			}
			if !exists {
				existing = append(existing, map[string]interface{}{
					"email":        acc.Email,
					"password":     acc.Password,
					"clientId":     acc.ClientID,
					"refreshToken": acc.RefreshToken,
					"registered":   false,
					"success":      false,
					"addedAt":      now,
				})
				addedCount++
			}
		}
		return existing
	})

	return map[string]interface{}{
		"added": addedCount,
		"total": len(storage.GetAccountsCached()),
	}
}

// GetOutlookAccounts 获取 Outlook 账号列表
func GetOutlookAccounts() []map[string]interface{} {
	return storage.GetAccountsCached()
}

// UpdateAccountStatus 更新账号注册状态（纯内存操作，异步刷盘）
// failReason 记录最近一次失败的分类原因；成功或无失败时传空字符串，会清除旧原因。
func UpdateAccountStatus(email string, registered bool, success bool, failReason string) map[string]interface{} {
	found := false
	now := time.Now().Format("2006-01-02 15:04:05")
	storage.ModifyAccountsCached(func(accounts []map[string]interface{}) []map[string]interface{} {
		for i, acc := range accounts {
			if acc["email"] == email {
				accounts[i]["registered"] = registered
				accounts[i]["success"] = success
				accounts[i]["registeredAt"] = now
				// 成功时清除失败原因，失败时记录最近一次原因
				if failReason == "" {
					delete(accounts[i], "failReason")
				} else {
					accounts[i]["failReason"] = failReason
				}
				found = true
				break
			}
		}
		return accounts
	})
	if !found {
		return map[string]interface{}{"error": "账号不存在"}
	}
	return map[string]interface{}{"status": "updated"}
}

// MarkAccountFailReason 仅记录账号最近一次失败原因，不改变 registered/success。
// 用于验证码等前置阶段失败：邮箱仍可被下次任务领取重试（registered 保持 false），
// 但需保留失败原因供前端筛选展示。
func MarkAccountFailReason(email string, failReason string) map[string]interface{} {
	if failReason == "" {
		return map[string]interface{}{"status": "skipped"}
	}
	found := false
	now := time.Now().Format("2006-01-02 15:04:05")
	storage.ModifyAccountsCached(func(accounts []map[string]interface{}) []map[string]interface{} {
		for i, acc := range accounts {
			if acc["email"] == email {
				if failReason == "验证码超时" && acc["failReason"] == "验证码超时" {
					accounts[i]["registered"] = true
					accounts[i]["success"] = false
					accounts[i]["registeredAt"] = now
					accounts[i]["failReason"] = "异常邮箱"
				} else {
					accounts[i]["failReason"] = failReason
				}
				found = true
				break
			}
		}
		return accounts
	})
	if !found {
		return map[string]interface{}{"error": "账号不存在"}
	}
	return map[string]interface{}{"status": "updated"}
}

// SaveOutlookGraphResolution 持久化单个 Outlook 账号的 Graph 地址解析结果。
// 仅保存映射相关字段，大小写不敏感匹配导入邮箱，不修改注册状态或凭据。
func SaveOutlookGraphResolution(importedEmail string, resolved OutlookAccount) map[string]interface{} {
	key := strings.ToLower(strings.TrimSpace(importedEmail))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(resolved.Email))
	}
	if key == "" {
		return map[string]interface{}{"error": "邮箱为空"}
	}
	found := false
	now := time.Now().Format("2006-01-02 15:04:05")
	storage.ModifyAccountsCached(func(accounts []map[string]interface{}) []map[string]interface{} {
		for i := range accounts {
			if accounts[i] == nil {
				continue
			}
			emailValue, _ := accounts[i]["email"].(string)
			if strings.ToLower(strings.TrimSpace(emailValue)) != key {
				continue
			}
			if strings.TrimSpace(resolved.RegistrationEmail) != "" {
				accounts[i]["registrationEmail"] = strings.TrimSpace(resolved.RegistrationEmail)
			}
			if strings.TrimSpace(resolved.GraphPrimaryEmail) != "" {
				accounts[i]["graphPrimaryEmail"] = strings.TrimSpace(resolved.GraphPrimaryEmail)
			}
			accounts[i]["graphAliasVerified"] = resolved.GraphAliasVerified
			if strings.TrimSpace(resolved.GraphResolvedAt) != "" {
				accounts[i]["graphResolvedAt"] = strings.TrimSpace(resolved.GraphResolvedAt)
			} else {
				accounts[i]["graphResolvedAt"] = now
			}
			found = true
			break
		}
		return accounts
	})
	if !found {
		return map[string]interface{}{"error": "账号不存在"}
	}
	return map[string]interface{}{"status": "updated"}
}

// DeleteOutlookAccount 删除单个 Outlook 账号（纯内存操作，异步刷盘）
func DeleteOutlookAccount(email string) map[string]interface{} {
	found := false
	newLen := 0
	storage.ModifyAccountsCached(func(accounts []map[string]interface{}) []map[string]interface{} {
		newAccounts := make([]map[string]interface{}, 0, len(accounts))
		for _, acc := range accounts {
			if acc["email"] == email {
				found = true
				continue
			}
			newAccounts = append(newAccounts, acc)
		}
		newLen = len(newAccounts)
		return newAccounts
	})
	if !found {
		return map[string]interface{}{"error": "账号不存在"}
	}
	return map[string]interface{}{
		"status": "deleted",
		"total":  newLen,
	}
}

// DeleteOutlookAccounts 批量删除多个 Outlook 账号（纯内存操作，异步刷盘）
func DeleteOutlookAccounts(emails []string) map[string]interface{} {
	// 先把待删邮箱放入 set，匹配复杂度降到 O(1)，整体删除为 O(n)
	target := make(map[string]struct{}, len(emails))
	for _, e := range emails {
		target[e] = struct{}{}
	}
	removed := 0
	newLen := 0
	storage.ModifyAccountsCached(func(accounts []map[string]interface{}) []map[string]interface{} {
		out := make([]map[string]interface{}, 0, len(accounts))
		for _, acc := range accounts {
			if em, _ := acc["email"].(string); em != "" {
				if _, ok := target[em]; ok {
					removed++
					continue
				}
			}
			out = append(out, acc)
		}
		newLen = len(out)
		return out
	})
	return map[string]interface{}{"status": "deleted", "removed": removed, "total": newLen}
}

// ClearOutlookAccounts 清空所有 Outlook 账号
func ClearOutlookAccounts() map[string]interface{} {
	storage.SetAccountsCached([]map[string]interface{}{})
	return map[string]interface{}{"status": "cleared"}
}

// ClearRegisteredOutlookAccounts 仅清除已标记为已注册的账号（成功/失败均算）
func ClearRegisteredOutlookAccounts() map[string]interface{} {
	removed := 0
	newLen := 0
	storage.ModifyAccountsCached(func(accounts []map[string]interface{}) []map[string]interface{} {
		out := make([]map[string]interface{}, 0, len(accounts))
		for _, acc := range accounts {
			if reg, _ := acc["registered"].(bool); reg {
				removed++
				continue
			}
			out = append(out, acc)
		}
		newLen = len(out)
		return out
	})
	return map[string]interface{}{"status": "ok", "removed": removed, "total": newLen}
}

// ResetOutlookAccountStatuses 将所有 Outlook 账号恢复为未注册状态。
func ResetOutlookAccountStatuses() map[string]interface{} {
	reset := 0
	storage.ModifyAccountsCached(func(accounts []map[string]interface{}) []map[string]interface{} {
		for i := range accounts {
			if accounts[i] == nil {
				continue
			}
			resetOutlookStatusFields(accounts[i])
			reset++
		}
		return accounts
	})
	return map[string]interface{}{"status": "reset", "reset": reset, "total": reset}
}

// ResetOutlookAccountStatusesByEmails 将指定 Outlook 账号恢复为未注册状态。
// 只重置注册状态相关字段，保留邮箱、密码、ClientID 与 RefreshToken 等凭据字段。
func ResetOutlookAccountStatusesByEmails(emails []string) map[string]interface{} {
	target := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			target[email] = struct{}{}
		}
	}
	if len(target) == 0 {
		return map[string]interface{}{"status": "reset", "reset": 0}
	}

	reset := 0
	storage.ModifyAccountsCached(func(accounts []map[string]interface{}) []map[string]interface{} {
		for i := range accounts {
			if accounts[i] == nil {
				continue
			}
			email, _ := accounts[i]["email"].(string)
			if _, ok := target[strings.ToLower(strings.TrimSpace(email))]; !ok {
				continue
			}
			resetOutlookStatusFields(accounts[i])
			reset++
		}
		return accounts
	})
	return map[string]interface{}{"status": "reset", "reset": reset}
}

func resetOutlookStatusFields(account map[string]interface{}) {
	// 只重置状态字段，保留账号、密码、ClientID 与 RefreshToken。
	account["registered"] = false
	account["success"] = false
	delete(account, "registeredAt")
	delete(account, "failReason")
}

// ImportOutlookFile 导入 Outlook 账号文件
func ImportOutlookFile(filePath string) map[string]interface{} {
	if filePath == "" {
		return map[string]interface{}{"error": "未选择文件"}
	}

	// 读取文件内容
	data, err := os.ReadFile(filePath)
	if err != nil {
		return map[string]interface{}{"error": "读取文件失败: " + err.Error()}
	}

	// 使用现有的解析和添加逻辑
	return AddOutlookAccounts(string(data))
}
