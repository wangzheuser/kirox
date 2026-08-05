package core

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"reg_go/internal/browser"
	"reg_go/internal/email"
	httputil "reg_go/internal/http"
)

// shortResponseBody 截断响应体，避免失败日志过长
func shortResponseBody(body []byte, limit int) string {
	text := strings.TrimSpace(string(body))
	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}

// Step6SubmitEmail 提交邮箱
func (r *Registrar) Step6SubmitEmail() (string, error) {
	log.Printf("[6] 提交邮箱 %s", r.Email)
	api := fmt.Sprintf("%s/platform/%s/api/execute", r.Cfg.SigninBase, r.Cfg.DirectoryID)
	ref := fmt.Sprintf("%s/platform/%s/login?workflowStateHandle=%s",
		r.Cfg.SigninBase, r.Cfg.DirectoryID, r.WorkflowHandle)
	fp := r.GenFP("signin", "PageSubmit", len(r.Email), r.Email)

	rid := NewUUID()
	h := r.BuildHeaders(ref, r.Cfg.SigninBase)
	h["x-amzn-requestid"] = rid
	h["x-amz-date"] = GmtDate()
	h["priority"] = "u=1, i"

	body, status, respH, err := r.DoPostRaw(api, orderedExecutePayload(
		"get-identity-user", r.WorkflowHandle, "SUBMIT",
		[]interface{}{
			orderedUserRequestInput(r.Email),
			orderedApplicationTypeRequestInput("SSO_INDIVIDUAL_ID"),
			orderedUserEventRequestInput(r.Cfg.DirectoryID, r.Email, "PAGE_SUBMIT", "IDENTIFICATION", 5000),
			orderedFingerPrintRequestInput(fp),
		},
		r.VisitorID, rid,
	), h)
	if err != nil {
		return "", err
	}
	httputil.SaveCookies(r.Cookies, respH)

	var data map[string]interface{}
	_ = json.Unmarshal(body, &data)

	if status == 400 {
		if isTESBlockedResponse(body) {
			return "", fmt.Errorf("提交邮箱失败: %d - %s", status, shortResponseBody(body, 500))
		}
		workflowHandle := signupWorkflowHandle(data)
		if workflowHandle == "" {
			return "", fmt.Errorf("提交邮箱失败: %d - 未返回有效 signup redirect/workflowStateHandle: %s", status, shortResponseBody(body, 500))
		}
		r.WorkflowHandle = workflowHandle
		return "signup", nil
	} else if status == 200 {
		if wh, ok := data["workflowStateHandle"].(string); ok && strings.TrimSpace(wh) != "" {
			r.WorkflowHandle = strings.TrimSpace(wh)
		}
		return "login", nil
	}
	return "", fmt.Errorf("提交邮箱失败: %d - %s", status, string(body)[:min(200, len(body))])
}

func signupWorkflowHandle(data map[string]interface{}) string {
	responseHandle, _ := data["workflowStateHandle"].(string)
	responseHandle = strings.TrimSpace(responseHandle)
	rawRedirect, hasRedirect := data["redirect"]
	if !hasRedirect {
		message, _ := data["message"].(map[string]interface{})
		errorCode, _ := message["errorCode"].(string)
		if !strings.EqualFold(strings.TrimSpace(errorCode), "ENTITY_DOES_NOT_EXIST") {
			return ""
		}
		return responseHandle
	}
	redirect, ok := rawRedirect.(map[string]interface{})
	if !ok {
		return ""
	}
	rawURL, _ := redirect["url"].(string)
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/signup") {
		return ""
	}
	handle := strings.TrimSpace(parsed.Query().Get("workflowStateHandle"))
	if handle == "" {
		return ""
	}
	if responseHandle != "" && responseHandle != handle {
		return ""
	}
	return handle
}

func isTESBlockedResponse(body []byte) bool {
	lower := strings.ToLower(strings.TrimSpace(string(body)))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, `"errorcode":"blocked"`) ||
		strings.Contains(lower, `"errorcode": "blocked"`) ||
		strings.Contains(lower, "request was blocked by tes") ||
		strings.Contains(lower, "注册请求被拦截")
}

// Step7Signup 注册
func (r *Registrar) Step7Signup() error {
	log.Println("[7] 注册 (SIGNUP)")
	api := fmt.Sprintf("%s/platform/%s/api/execute", r.Cfg.SigninBase, r.Cfg.DirectoryID)
	ref := fmt.Sprintf("%s/platform/%s/login?workflowStateHandle=%s",
		r.Cfg.SigninBase, r.Cfg.DirectoryID, r.WorkflowHandle)
	fp := r.GenFP("signup", "PageSubmit", 0, "")

	rid := NewUUID()
	h := r.BuildHeaders(ref, r.Cfg.SigninBase)
	h["x-amzn-requestid"] = rid
	h["x-amz-date"] = GmtDate()
	h["priority"] = "u=1, i"

	body, _, respH, err := r.DoPostRaw(api, orderedExecutePayload(
		"get-identity-user", r.WorkflowHandle, "SIGNUP",
		[]interface{}{
			orderedUserRequestInput(r.Email),
			orderedFingerPrintRequestInput(fp),
		},
		r.VisitorID, rid,
	), h)
	if err != nil {
		return err
	}
	httputil.SaveCookies(r.Cookies, respH)

	var data map[string]interface{}
	json.Unmarshal(body, &data)
	if redir, ok := data["redirect"].(map[string]interface{}); ok {
		if rurl, ok := redir["url"].(string); ok && strings.Contains(rurl, "workflowStateHandle=") {
			r.WorkflowHandle = httputil.SplitAfter(rurl, "workflowStateHandle=")
		}
	}
	return nil
}

// Step7_5SignupInit Signup API 初始化
func (r *Registrar) Step7_5SignupInit() error {
	log.Println("[7.5] Signup API 初始化")
	api := fmt.Sprintf("%s/platform/%s/signup/api/execute", r.Cfg.SigninBase, r.Cfg.DirectoryID)
	ref := fmt.Sprintf("%s/platform/%s/signup?workflowStateHandle=%s",
		r.Cfg.SigninBase, r.Cfg.DirectoryID, r.WorkflowHandle)

	fp := r.GenFP("signup", "first_load", 0, "")
	rid := NewUUID()
	h := r.BuildHeaders(ref, r.Cfg.SigninBase)
	h["x-amzn-requestid"] = rid
	h["x-amz-date"] = GmtDate()
	h["priority"] = "u=1, i"

	body, _, respH, err := r.DoPostRaw(api, orderedExecutePayload(
		"", r.WorkflowHandle, "",
		[]interface{}{
			orderedUserRequestInput(r.Email),
			orderedFingerPrintRequestInput(fp),
		},
		r.VisitorID, rid,
	), h)
	if err != nil {
		return err
	}
	httputil.SaveCookies(r.Cookies, respH)

	var data map[string]interface{}
	json.Unmarshal(body, &data)
	if wh, ok := data["workflowStateHandle"].(string); ok {
		r.WorkflowHandle = wh
	}
	if data["stepId"] != "start" {
		return fmt.Errorf("Signup init 返回意外 stepId: %v", data["stepId"])
	}

	// 第二次请求
	fp = r.GenFP("signup", "PageLoad", 0, "")
	rid = NewUUID()
	h = r.BuildHeaders(ref, r.Cfg.SigninBase)
	h["x-amzn-requestid"] = rid
	h["x-amz-date"] = GmtDate()
	h["priority"] = "u=1, i"

	body, _, respH, err = r.DoPostRaw(api, orderedExecutePayload(
		"start", r.WorkflowHandle, "",
		[]interface{}{
			orderedUserRequestInput(r.Email),
			orderedFingerPrintRequestInput(fp),
		},
		r.VisitorID, rid,
	), h)
	if err != nil {
		return err
	}
	httputil.SaveCookies(r.Cookies, respH)

	json.Unmarshal(body, &data)
	if wh, ok := data["workflowStateHandle"].(string); ok {
		r.WorkflowHandle = wh
	}
	if redir, ok := data["redirect"].(map[string]interface{}); ok {
		if rurl, ok := redir["url"].(string); ok && strings.Contains(rurl, "workflowID=") {
			wid := httputil.SplitAfter(rurl, "workflowID=")
			if i := strings.IndexByte(wid, '#'); i >= 0 {
				wid = wid[:i]
			}
			r.WorkflowID = wid
		}
	}
	if r.WorkflowID == "" {
		return fmt.Errorf("Signup init 未返回 workflowID")
	}
	return nil
}

// Step7_8ProfileInit Profile 页面初始化
func (r *Registrar) Step7_8ProfileInit() error {
	log.Println("[7.8] Profile 页面初始化")
	r.Ubid = httputil.UbidGen()
	r.Cookies["aws-user-profile-ubid"] = r.Ubid
	r.Cookies["i18next"] = r.Cfg.BrowserLocale().I18Next
	if _, ok := r.Cookies["awsccc"]; !ok {
		r.Cookies["awsccc"] = httputil.Awsccc()
	}

	url := fmt.Sprintf("%s/?workflowID=%s", r.Cfg.ProfileBase, r.WorkflowID)
	body, _, respH, err := r.DoGet(url, r.BuildDocumentHeaders())
	if err != nil {
		return err
	}
	if r.FPCtx != nil {
		r.FPCtx.SetProfileHTML(string(body))
	}
	httputil.SaveCookies(r.Cookies, respH)
	r.FPCtx.ResetPerfTiming()
	return r.FetchD2CToken(r.Cfg.ProfileBase, url)
}

// Step8ProfileStart Profile 启动
func (r *Registrar) Step8ProfileStart() error {
	log.Println("[8] Profile 启动")
	ref := fmt.Sprintf("%s/?workflowID=%s", r.Cfg.ProfileBase, r.WorkflowID)
	fp := r.GenFP("profile", "PageLoad", 0, "")

	attrs := browser.NewOrderedMap()
	attrs.Set("fingerprint", fp)
	attrs.Set("eventTimestamp", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
	attrs.Set("timeSpentOnPage", "38")
	attrs.Set("eventType", "PageLoad")
	attrs.Set("ubid", r.Ubid)
	attrs.Set("visitorId", r.VisitorID)

	browserData := browser.NewOrderedMap()
	browserData.Set("attributes", attrs)
	browserData.Set("cookies", browser.NewOrderedMap())

	reqPayload := browser.NewOrderedMap()
	reqPayload.Set("workflowID", r.WorkflowID)
	reqPayload.Set("browserData", browserData)

	body, _, _, err := r.DoPostRaw(r.Cfg.ProfileBase+"/api/start", reqPayload, r.BuildProfileHeaders(ref))
	if err != nil {
		return err
	}

	var data map[string]interface{}
	json.Unmarshal(body, &data)
	r.WorkflowState, _ = data["workflowState"].(string)
	if r.WorkflowState == "" {
		return fmt.Errorf("Profile start 未返回 workflowState: %s", string(body))
	}
	if len(r.WorkflowState) > 30 {
		log.Printf("workflowState=%s...", r.WorkflowState[:30])
	}
	return nil
}

// Step9SendOTP 发送验证码
func (r *Registrar) Step9SendOTP() error {
	ref := fmt.Sprintf("%s/?workflowID=%s", r.Cfg.ProfileBase, r.WorkflowID)

	// Outlook 模式: 根据读取方式记录发送验证码前的定位信息。
	if r.Cfg.UseOutlook && r.Cfg.OutlookAccount != nil {
		if r.Cfg.UseOutlookGraph() {
			r.Cfg.OutlookOTPAfter = time.Now().UTC().Add(-5 * time.Second)
			log.Printf("[Outlook Graph] 记录验证码起始时间: %s", r.Cfg.OutlookOTPAfter.Format(time.RFC3339))
		} else {
			count, err := email.GetInboxCountWithProxy(*r.Cfg.OutlookAccount, r.Cfg.EmailProxy)
			if err != nil {
				log.Printf("获取邮件数量失败: %v, 默认为0", err)
			} else {
				r.OutlookMailCount = count
				log.Printf("发送前邮件数: %d", count)
			}
		}
	}

	log.Println("[9] 发送验证码")
	fp := r.GenFP("profile", "PageSubmit", len(r.Email), r.Email)

	attrs := browser.NewOrderedMap()
	attrs.Set("fingerprint", fp)
	attrs.Set("eventTimestamp", time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
	attrs.Set("timeSpentOnPage", "0")
	attrs.Set("pageName", "EMAIL_COLLECTION")
	attrs.Set("eventType", "PageSubmit")
	attrs.Set("ubid", r.Ubid)
	attrs.Set("visitorId", r.VisitorID)

	browserData := browser.NewOrderedMap()
	browserData.Set("attributes", attrs)
	browserData.Set("cookies", browser.NewOrderedMap())

	reqPayload := browser.NewOrderedMap()
	reqPayload.Set("workflowState", r.WorkflowState)
	reqPayload.Set("email", r.Email)
	reqPayload.Set("browserData", browserData)

	// 完整 profile 表单已经构造并进入提交步骤；响应是否接受由 OTPSent 区分。
	r.FormSubmitted = true
	respBody, status, _, err := r.DoPostRaw(r.Cfg.ProfileBase+"/api/send-otp", reqPayload, r.BuildProfileHeaders(ref))
	if err != nil {
		return err
	}
	if status != 200 {
		bodyText := shortResponseBody(respBody, 500)
		diagnostics := sendOTPFailureContext(r.Cfg, r.Email)
		log.Printf("[send-otp] 失败: status=%d, body=%s, fp_len=%d, %s", status, bodyText, len(fp), diagnostics)
		if bodyText != "" {
			return fmt.Errorf("send-otp 失败 (%d): %s [%s]", status, bodyText, diagnostics)
		}
		return fmt.Errorf("send-otp 失败 (%d) [%s]", status, diagnostics)
	}
	r.OTPSent = true
	log.Println("验证码已发送")
	return nil
}

func sendOTPFailureContext(cfg *Config, emailAddr string) string {
	provider := "<unknown>"
	emailProxyState := "direct"
	proxyState := "direct"
	locale := DefaultBrowserLocale()
	if cfg != nil {
		if strings.TrimSpace(cfg.EmailProvider) != "" {
			provider = strings.TrimSpace(cfg.EmailProvider)
		}
		if strings.TrimSpace(cfg.EmailProxy) != "" {
			emailProxyState = "enabled"
		}
		if strings.TrimSpace(cfg.Proxy) != "" {
			proxyState = "enabled"
		}
		locale = cfg.BrowserLocale()
	}
	domain := "<unknown>"
	if at := strings.LastIndex(strings.TrimSpace(emailAddr), "@"); at >= 0 && at+1 < len(strings.TrimSpace(emailAddr)) {
		domain = strings.ToLower(strings.TrimSpace(emailAddr)[at+1:])
	}
	return fmt.Sprintf("provider=%s, domain=%s, emailProxy=%s, proxy=%s, acceptLanguage=%s, i18next=%s, timeZone=%d",
		provider, domain, emailProxyState, proxyState, primaryLanguageTag(locale.AcceptLanguage), locale.I18Next, locale.TimeZone)
}

func primaryLanguageTag(acceptLanguage string) string {
	acceptLanguage = strings.TrimSpace(acceptLanguage)
	if acceptLanguage == "" {
		return "<unknown>"
	}
	if comma := strings.IndexByte(acceptLanguage, ','); comma >= 0 {
		acceptLanguage = acceptLanguage[:comma]
	}
	if semi := strings.IndexByte(acceptLanguage, ';'); semi >= 0 {
		acceptLanguage = acceptLanguage[:semi]
	}
	acceptLanguage = strings.TrimSpace(acceptLanguage)
	if acceptLanguage == "" {
		return "<unknown>"
	}
	return acceptLanguage
}

// Step10GetOTP 等待验证码 (临时邮箱或 Outlook)
func (r *Registrar) Step10GetOTP() (string, error) {
	log.Println("[10] 等待验证码")
	if r.Cfg.UseOutlook && r.Cfg.OutlookAccount != nil {
		var (
			code string
			err  error
		)
		if r.Cfg.UseOutlookGraph() {
			code, err = email.WaitForOTPGraphWithProxy(r.Ctx, *r.Cfg.OutlookAccount, r.Cfg.OutlookOTPAfter, r.Cfg.OTPTimeout, 5, r.Cfg.EmailProxy)
		} else {
			code, err = email.WaitForOTPWithProxy(r.Ctx, *r.Cfg.OutlookAccount, r.OutlookMailCount, r.Cfg.OTPTimeout, 5, r.Cfg.EmailProxy)
		}
		if err != nil {
			return "", err
		}
		log.Printf("验证码: %s", code)
		return code, nil
	}
	code, err := r.EmailSvc.WaitForCode(r.Cfg.OTPTimeout, 3)
	if err != nil {
		return "", err
	}
	log.Printf("验证码: %s", code)
	return code, nil
}
