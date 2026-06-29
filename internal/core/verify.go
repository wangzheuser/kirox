package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	httputil "reg_go/internal/http"
)

type verifyHTTPClient interface {
	Do(req *fhttp.Request) (*fhttp.Response, error)
}

var newVerifyHTTPClient = func(cfg *Config, chromeVer string) verifyHTTPClient {
	proxy := ""
	if cfg != nil {
		proxy = cfg.Proxy
	}
	return httputil.NewTLSClient(proxy, true, chromeVer)
}

// VerifyAlive 验活: 刷新 Token + 查用量 + 查模型
func (r *Registrar) VerifyAlive(awsToken map[string]interface{}) map[string]interface{} {
	log.Println("[验活] 刷新 Token + 查用量 + 查模型")
	chromeVer := ""
	if r.Identity != nil {
		chromeVer = r.Identity.ChromeVer
	}
	client := newVerifyHTTPClient(r.Cfg, chromeVer)

	refreshToken, _ := awsToken["refreshToken"].(string)

	tokenBody, _ := json.Marshal(map[string]string{
		"clientId":     r.ClientID,
		"clientSecret": r.ClientSecret,
		"refreshToken": refreshToken,
		"grantType":    "refresh_token",
	})
	req, _ := fhttp.NewRequest("POST", verifyOIDCTokenURL(r.Cfg),
		bytes.NewReader(tokenBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("验活异常: %v", err)
		return map[string]interface{}{"alive": false, "error": err.Error()}
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Token 刷新失败: %d", resp.StatusCode)
		return map[string]interface{}{"alive": false, "error": fmt.Sprintf("refresh failed: %d", resp.StatusCode)}
	}

	var tok map[string]interface{}
	json.Unmarshal(body, &tok)
	access, _ := tok["accessToken"].(string)
	expiresIn, _ := tok["expiresIn"].(float64)
	log.Printf("Token 刷新成功, expiresIn=%ds", int(expiresIn))

	usageURL := verifyQEndpointURL(r.Cfg, "/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true")
	usageRes := queryGetEndpoint(client, access, usageURL)
	if usageRes.suspended {
		return map[string]interface{}{"alive": false, "suspended": true, "error": "suspended"}
	}
	if !usageRes.ok {
		return map[string]interface{}{"alive": false, "error": "usage query failed"}
	}

	modelRes := queryGetEndpoint(client, access, verifyQEndpointURL(r.Cfg, "/ListAvailableModels?origin=AI_EDITOR"))
	if modelRes.suspended {
		return map[string]interface{}{"alive": false, "suspended": true, "error": "suspended"}
	}
	if !modelRes.ok {
		return map[string]interface{}{"alive": false, "error": fmt.Sprintf("models query failed: %d", modelRes.statusCode)}
	}

	return r.parseUsage(usageRes.body)
}

func verifyOIDCTokenURL(cfg *Config) string {
	base := "https://oidc.us-east-1.amazonaws.com"
	if cfg != nil && strings.TrimSpace(cfg.OIDCBase) != "" {
		base = strings.TrimSpace(cfg.OIDCBase)
	}
	return strings.TrimRight(base, "/") + "/token"
}

func verifyQEndpointURL(cfg *Config, path string) string {
	base := "https://q.us-east-1.amazonaws.com"
	if cfg != nil && strings.TrimSpace(cfg.QBase) != "" {
		base = strings.TrimSpace(cfg.QBase)
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(base, "/") + path
}

type endpointResult struct {
	body       []byte
	ok         bool
	suspended  bool
	statusCode int
}

func checkEndpointResponse(url string, statusCode int, body []byte) endpointResult {
	label := endpointLabel(url)
	if statusCode == 403 {
		if label == "models" {
			if responseIndicatesSuspension(body) {
				log.Printf("账号已被封禁 (403) [%s]: body=%s", label, endpointBodySnippet(body))
				return endpointResult{suspended: true, statusCode: statusCode}
			}
			if snippet := endpointBodySnippet(body); snippet != "" {
				log.Printf("端点查询失败 [%s]: %d body=%s", label, statusCode, snippet)
			} else {
				log.Printf("端点查询失败 [%s]: %d", label, statusCode)
			}
			return endpointResult{statusCode: statusCode}
		}
		log.Printf("账号已被封禁 (403) [%s]", label)
		return endpointResult{suspended: true, statusCode: statusCode}
	}
	if statusCode != 200 {
		if label == "usage" {
			if reason := responseReason(body); reason != "" {
				log.Printf("账号已被封禁 (reason) [%s]: status=%d reason=%s body=%s", label, statusCode, reason, string(body))
				return endpointResult{suspended: true, statusCode: statusCode}
			}
		}
		log.Printf("端点查询失败 [%s]: %d", label, statusCode)
		return endpointResult{statusCode: statusCode}
	}
	return endpointResult{body: body, ok: true, statusCode: statusCode}
}

func endpointBodySnippet(body []byte) string {
	text := strings.TrimSpace(scrubURLs(string(body)))
	if text == "" {
		return ""
	}
	const maxLen = 200
	if len(text) > maxLen {
		text = text[:maxLen] + "..."
	}
	return text
}

func responseIndicatesSuspension(body []byte) bool {
	lower := strings.ToLower(string(body))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "suspended") ||
		strings.Contains(lower, "locked your account") ||
		strings.Contains(lower, "account locked") ||
		strings.Contains(lower, "security precaution") ||
		strings.Contains(lower, "account_suspended")
}

func responseReason(body []byte) string {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	reason, _ := payload["reason"].(string)
	return strings.TrimSpace(reason)
}

// endpointLabel 把完整 URL 归一到简短标签，避免日志泄露后端。
func endpointLabel(url string) string {
	switch {
	case strings.Contains(url, "getUsageLimits"):
		return "usage"
	case strings.Contains(url, "ListAvailableModels"):
		return "models"
	case strings.Contains(url, "refreshToken"):
		return "kiro-refresh"
	case strings.Contains(url, "/token"):
		return "oidc-token"
	default:
		return "endpoint"
	}
}

func queryGetEndpoint(client interface {
	Do(req *fhttp.Request) (*fhttp.Response, error)
}, access, url string) endpointResult {
	req, _ := fhttp.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("User-Agent", "aws-sdk-js/1.0.18 ua/2.1 os/windows lang/js md/nodejs#20.16.0 api/codewhispererstreaming#1.0.18 m/E KiroIDE-0.6.18")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("端点查询异常 [%s]: %s", endpointLabel(url), scrubURLs(err.Error()))
		return endpointResult{}
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return checkEndpointResponse(url, resp.StatusCode, body)
}

func (r *Registrar) parseUsage(body []byte) map[string]interface{} {
	var usage map[string]interface{}
	json.Unmarshal(body, &usage)

	userInfo, _ := usage["userInfo"].(map[string]interface{})
	emailAddr, _ := userInfo["email"].(string)
	subInfo, _ := usage["subscriptionInfo"].(map[string]interface{})
	sub, _ := subInfo["subscriptionTitle"].(string)
	if sub == "" {
		sub = "Free"
	}

	var totalLimit, totalUsed float64
	if breakdown, ok := usage["usageBreakdownList"].([]interface{}); ok {
		for _, item := range breakdown {
			b, _ := item.(map[string]interface{})
			rt, _ := b["resourceType"].(string)
			dn, _ := b["displayName"].(string)
			if rt == "CREDIT" || dn == "Credits" {
				baseLimit, _ := b["usageLimitWithPrecision"].(float64)
				if baseLimit == 0 {
					baseLimit, _ = b["usageLimit"].(float64)
				}
				baseUsed, _ := b["currentUsageWithPrecision"].(float64)
				if baseUsed == 0 {
					baseUsed, _ = b["currentUsage"].(float64)
				}
				totalLimit = baseLimit
				totalUsed = baseUsed

				if ft, ok := b["freeTrialInfo"].(map[string]interface{}); ok {
					if ftStatus, _ := ft["freeTrialStatus"].(string); ftStatus == "ACTIVE" {
						ftLimit, _ := ft["usageLimitWithPrecision"].(float64)
						ftUsed, _ := ft["currentUsageWithPrecision"].(float64)
						totalLimit += ftLimit
						totalUsed += ftUsed
					}
				}
				break
			}
		}
	}

	log.Printf("验活成功! 邮箱=%s 订阅=%s Credit=%.1f/%.1f", emailAddr, sub, totalUsed, totalLimit)
	return map[string]interface{}{
		"alive": true, "email": emailAddr, "subscription": sub,
		"credit_used": totalUsed, "credit_limit": totalLimit,
	}
}
