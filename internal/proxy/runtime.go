package proxy

import (
	"strings"

	"github.com/google/uuid"
)

const uuidPlaceholder = "{uuid}"

// RenderURLTemplate 将代理地址中的运行时占位符渲染为本次使用的真实值。
func RenderURLTemplate(raw string) string {
	return renderURLTemplate(raw, uuid.NewString)
}

// HasURLTemplate 判断代理地址是否包含运行时占位符。
func HasURLTemplate(raw string) bool {
	return strings.Contains(strings.TrimSpace(raw), uuidPlaceholder)
}

// renderURLTemplate 执行实际渲染逻辑，uuidFactory 仅用于单元测试注入固定 UUID。
func renderURLTemplate(raw string, uuidFactory func() string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.Contains(trimmed, uuidPlaceholder) {
		return trimmed
	}

	// 同一个代理 URL 中的多个 {uuid} 必须使用同一个值，确保代理会话一致。
	sessionID := uuidFactory()
	return strings.ReplaceAll(trimmed, uuidPlaceholder, sessionID)
}
