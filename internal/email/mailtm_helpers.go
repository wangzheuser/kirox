package email

import (
	"encoding/json"
	"fmt"
	"strings"
)

func decodeMailTMDomains(body []byte) ([]string, error) {
	var payload interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析 mail.tm 域名失败: %w", err)
	}
	return normalizeMailGWDomains(payload), nil
}

func decodeMailTMToken(body []byte) (string, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("解析 token 响应失败: %w", err)
	}
	token := strings.TrimSpace(fmt.Sprint(payload["token"]))
	if token == "" || token == "<nil>" {
		return "", fmt.Errorf("token 响应缺少 token")
	}
	return token, nil
}
