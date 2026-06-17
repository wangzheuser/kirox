package browser

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math/rand"
	"strings"

	"reg_go/internal/crypto"
)

var perfTimingOrder = []string{
	"connectStart", "secureConnectionStart", "unloadEventEnd",
	"domainLookupStart", "domainLookupEnd", "responseStart",
	"connectEnd", "responseEnd", "requestStart", "domLoading",
	"redirectStart", "loadEventEnd", "domComplete", "navigationStart",
	"loadEventStart", "domContentLoadedEventEnd", "unloadEventStart",
	"redirectEnd", "domInteractive", "fetchStart", "domContentLoadedEventStart",
}

// GenPerfTiming 生成 performance.timing
func GenPerfTiming(nowMs int64) map[string]int64 {
	loadEventEnd := nowMs - int64(500+rand.Intn(1001))
	loadDuration := int64(2000 + rand.Intn(2001))
	base := loadEventEnd - loadDuration

	dnsOffset := int64(2 + rand.Intn(8))
	connectEndOffset := int64(300 + rand.Intn(300))
	responseOffset := connectEndOffset + int64(200+rand.Intn(400))
	domInteractiveOffset := loadDuration - int64(5+rand.Intn(11))
	domContentLoadedStart := domInteractiveOffset + int64(rand.Intn(3))

	return map[string]int64{
		"connectStart":               base + dnsOffset + 1 + int64(rand.Intn(3)),
		"secureConnectionStart":      base + dnsOffset + 3 + int64(rand.Intn(5)),
		"unloadEventEnd":             0,
		"domainLookupStart":          base + dnsOffset,
		"domainLookupEnd":            base + dnsOffset + int64(rand.Intn(2)),
		"responseStart":              base + responseOffset,
		"connectEnd":                 base + connectEndOffset,
		"responseEnd":                base + responseOffset + int64(rand.Intn(5)),
		"requestStart":               base + connectEndOffset,
		"domLoading":                 base + responseOffset + 2 + int64(rand.Intn(5)),
		"redirectStart":              0,
		"loadEventEnd":               loadEventEnd,
		"domComplete":                loadEventEnd,
		"navigationStart":            base,
		"loadEventStart":             loadEventEnd,
		"domContentLoadedEventEnd":   loadEventEnd,
		"unloadEventStart":           0,
		"redirectEnd":                0,
		"domInteractive":             base + domInteractiveOffset,
		"fetchStart":                 base + dnsOffset,
		"domContentLoadedEventStart": base + domContentLoadedStart,
	}
}

func applyPreviousDocumentUnloadTiming(timing map[string]int64) {
	if timing == nil {
		return
	}
	navigationStart := timing["navigationStart"]
	responseEnd := timing["responseEnd"]
	if navigationStart == 0 || responseEnd <= navigationStart {
		return
	}
	if timing["unloadEventStart"] != 0 && timing["unloadEventEnd"] != 0 {
		return
	}

	unloadAt := timing["domainLookupEnd"] + int64(10+rand.Intn(10))
	if unloadAt < navigationStart {
		unloadAt = navigationStart
	}
	if unloadAt > responseEnd {
		unloadAt = responseEnd
	}
	timing["unloadEventStart"] = unloadAt
	timing["unloadEventEnd"] = unloadAt
}

func applyProfileBrowserLoadTimingShape(timing map[string]int64) {
	if timing == nil {
		return
	}
	base := timing["navigationStart"]
	if base == 0 {
		return
	}
	if loadEnd := timing["loadEventEnd"]; loadEnd > base && loadEnd-base <= 220 {
		return
	}

	lookupOffset := int64(1 + rand.Intn(3))
	requestOffset := int64(7 + rand.Intn(6))
	responseStartOffset := requestOffset + int64(2+rand.Intn(8))
	responseEndOffset := responseStartOffset + int64(8+rand.Intn(38))
	domLoadingOffset := responseStartOffset + int64(4+rand.Intn(10))
	domInteractiveOffset := int64(80 + rand.Intn(70))
	domContentLoadedStartOffset := domInteractiveOffset + int64(1+rand.Intn(4))
	domContentLoadedEndOffset := domContentLoadedStartOffset + int64(1+rand.Intn(4))
	loadEndOffset := domContentLoadedEndOffset + int64(1+rand.Intn(35))

	timing["connectStart"] = base + lookupOffset
	if timing["secureConnectionStart"] != 0 {
		timing["secureConnectionStart"] = base + lookupOffset + int64(2+rand.Intn(5))
	}
	timing["domainLookupStart"] = base + lookupOffset
	timing["domainLookupEnd"] = base + lookupOffset
	timing["connectEnd"] = base + lookupOffset
	timing["fetchStart"] = base + lookupOffset
	timing["requestStart"] = base + requestOffset
	timing["responseStart"] = base + responseStartOffset
	timing["responseEnd"] = base + responseEndOffset
	timing["domLoading"] = base + domLoadingOffset
	timing["domInteractive"] = base + domInteractiveOffset
	timing["domContentLoadedEventStart"] = base + domContentLoadedStartOffset
	timing["domContentLoadedEventEnd"] = base + domContentLoadedEndOffset
	timing["loadEventStart"] = base + loadEndOffset
	timing["loadEventEnd"] = base + loadEndOffset
	timing["domComplete"] = base + loadEndOffset
}

func formatScreen(s ScreenInfo) string {
	return fmt.Sprintf("%d-%d-%d-%d-*-*-*", s.Width, s.Height, s.AvailHeight, s.ColorDepth)
}

func formatPlugins(plugins []map[string]string) string {
	var sb strings.Builder
	for _, p := range plugins {
		sb.WriteString(p["name"])
		sb.WriteByte(' ')
		for _, r := range p["description"] {
			if r >= '0' && r <= '9' {
				sb.WriteRune(r)
			}
		}
	}
	return sb.String()
}

func genMetricsFirstLoad(pageType string) map[string]int {
	m := map[string]int{
		"el": 0, "script": 0, "h": 0, "batt": 0, "perf": 0, "auto": 0,
		"tz": 0, "fp2": 0, "lsubid": 0, "browser": 0, "capabilities": 0,
		"gpu": 0, "dnt": 0, "math": 0, "tts": 0, "input": 0, "canvas": 0,
		"captchainput": 0, "pow": 0,
	}
	switch pageType {
	case "profile":
		m["batt"] = 5 + rand.Intn(21)
		m["fp2"] = 1 + rand.Intn(8)
		m["browser"] = rand.Intn(4)
		m["capabilities"] = 1 + rand.Intn(8)
		m["dnt"] = rand.Intn(4)
		m["input"] = 8 + rand.Intn(23)
		m["canvas"] = 5 + rand.Intn(16)
	case "signup":
		m["script"] = rand.Intn(3)
		m["batt"] = rand.Intn(6)
		m["fp2"] = rand.Intn(4)
		m["gpu"] = 3 + rand.Intn(6)
	default:
		m["script"] = rand.Intn(3)
		m["auto"] = rand.Intn(3)
		m["browser"] = rand.Intn(3)
		m["gpu"] = 3 + rand.Intn(6)
	}
	return m
}

func orderedMetrics(values map[string]int) *OrderedMap {
	m := NewOrderedMap()
	for _, key := range []string{"el", "script", "h", "batt", "perf", "auto", "tz", "fp2", "lsubid", "browser", "capabilities", "gpu", "dnt", "math", "tts", "input", "canvas", "captchainput", "pow"} {
		m.Set(key, values[key])
	}
	return m
}

func orderedPerfTiming(values map[string]int64) *OrderedMap {
	m := NewOrderedMap()
	for _, key := range perfTimingOrder {
		m.Set(key, values[key])
	}
	return m
}

func orderedInteraction(clicks, touches, keyPresses, cuts, copies, pastes int, keyPressTimeIntervals []int, mouseClickPositions []string, keyCycles, mouseCycles, touchCycles []int) *OrderedMap {
	m := NewOrderedMap()
	m.Set("clicks", clicks)
	m.Set("touches", touches)
	m.Set("keyPresses", keyPresses)
	m.Set("cuts", cuts)
	m.Set("copies", copies)
	m.Set("pastes", pastes)
	m.Set("keyPressTimeIntervals", keyPressTimeIntervals)
	m.Set("mouseClickPositions", mouseClickPositions)
	m.Set("keyCycles", keyCycles)
	m.Set("mouseCycles", mouseCycles)
	m.Set("touchCycles", touchCycles)
	return m
}

func genMetricsPageSubmit() *OrderedMap {
	return orderedMetrics(map[string]int{
		"el": 0, "script": 0, "h": 0, "batt": 0, "perf": 0,
		"auto": 0, "tz": 0, "fp2": 0, "lsubid": 0, "browser": 0,
		"capabilities": 1, "gpu": 0, "dnt": 0, "math": 0, "tts": 0,
		"input": 0, "canvas": 0, "captchainput": 0, "pow": 0,
	})
}

// genInteraction 生成交互数据
func genInteraction(pageType, eventType string) *OrderedMap {
	if eventType == "PageLoad" || eventType == "first_load" {
		return orderedInteraction(0, 0, 0, 0, 0, 0, []int{}, []string{}, []int{}, []int{}, []int{})
	}
	if pageType == "profile" && eventType == "PageSubmit" {
		return orderedInteraction(
			1, 0, 1, 0, 0, 0,
			[]int{},
			[]string{fmt.Sprintf("%d,%d", 120+rand.Intn(61), 12+rand.Intn(17))},
			[]int{75 + rand.Intn(61)},
			[]int{},
			[]int{},
		)
	}
	nClicks := 1 + rand.Intn(10) // 1~10 clicks
	nKeys := 3 + rand.Intn(20)   // 3~22 keys
	nIntervals := max(1, nKeys/3) + rand.Intn(max(1, nKeys/2-nKeys/3+1))
	nCycles := max(2, nKeys/2) + rand.Intn(max(1, nKeys*2/3-nKeys/2+1))

	intervals := make([]int, nIntervals)
	for i := range intervals {
		intervals[i] = 30 + rand.Intn(1500) // 30ms-1.5s
	}
	cycles := make([]int, nCycles)
	for i := range cycles {
		cycles[i] = 10 + rand.Intn(800)
	}
	positions := make([]string, nClicks)
	for i := range positions {
		positions[i] = fmt.Sprintf("%d,%d", 50+rand.Intn(1500), 50+rand.Intn(800))
	}
	mouseCycles := make([]int, nClicks)
	for i := range mouseCycles {
		mouseCycles[i] = 20 + rand.Intn(300)
	}

	return orderedInteraction(nClicks, 0, nKeys, 0, 0, 0, intervals, positions, cycles, mouseCycles, []int{})
}

// genFormField 生成表单字段追踪数据
func genFormField(startMs int64, emailLen int, email string, interaction *OrderedMap) *OrderedMap {
	fieldTs := startMs - int64(10+rand.Intn(41))
	fieldRand := 1000 + rand.Intn(9000)
	fieldName := fmt.Sprintf("formField29-%d-%d", fieldTs, fieldRand)
	if strings.TrimSpace(email) != "" {
		fieldName = "email"
	}

	nKeys := max(3, emailLen/3+rand.Intn(10)-3)
	intervals := make([]int, min(nKeys-1, 10))
	for i := range intervals {
		intervals[i] = 30 + rand.Intn(1500)
	}
	keyCycles := make([]int, min(nKeys, 10))
	for i := range keyCycles {
		keyCycles[i] = 10 + rand.Intn(500)
	}

	// 如果有 interaction 数据，复用
	if kp, ok := interaction.Get("keyPresses"); ok {
		if kp, ok := kp.(int); ok && kp > 0 {
			nKeys = kp
		}
	}

	var cksum string
	if email != "" {
		cksum = fmt.Sprintf("%08X", crc32.ChecksumIEEE([]byte(email)))
	} else {
		cksum = fmt.Sprintf("%08X", crc32.ChecksumIEEE([]byte(fmt.Sprintf("user%d@example.com", 1000+rand.Intn(9000)))))
	}

	field := NewOrderedMap()
	field.Set("clicks", 1)
	field.Set("touches", 0)
	field.Set("keyPresses", nKeys)
	field.Set("cuts", 0)
	field.Set("copies", 0)
	field.Set("pastes", 0)
	mouseCycles := []int{80 + rand.Intn(71)}
	mouseClickPositions := []string{fmt.Sprintf("%d.5,%d.5", 100+rand.Intn(151), 10+rand.Intn(11))}
	totalFocusTime := 0
	width := 180
	height := 32
	if strings.TrimSpace(email) != "" {
		intervals = []int{}
		keyCycles = []int{75 + rand.Intn(61)}
		mouseCycles = []int{}
		mouseClickPositions = []string{fmt.Sprintf("%d,%d", 120+rand.Intn(61), 12+rand.Intn(17))}
		totalFocusTime = 900 + rand.Intn(5101)
		width = 188
		height = 38
	}
	field.Set("keyPressTimeIntervals", intervals)
	field.Set("mouseClickPositions", mouseClickPositions)
	field.Set("keyCycles", keyCycles)
	field.Set("mouseCycles", mouseCycles)
	field.Set("touchCycles", []int{})
	field.Set("width", width)
	field.Set("height", height)
	field.Set("totalFocusTime", totalFocusTime)
	field.Set("checksum", cksum)
	field.Set("autocomplete", false)
	field.Set("prefilled", strings.TrimSpace(email) != "")

	form := NewOrderedMap()
	form.Set(fieldName, field)
	return form
}

func orderedScripts(dynamicURLs []string, inlineHashes interface{}, scriptsElapsed, inlineHashesCount int) *OrderedMap {
	m := NewOrderedMap()
	m.Set("dynamicUrls", dynamicURLs)
	m.Set("inlineHashes", inlineHashes)
	m.Set("elapsed", scriptsElapsed)
	m.Set("dynamicUrlCount", len(dynamicURLs))
	m.Set("inlineHashesCount", inlineHashesCount)
	return m
}

func orderedHistory(length int) *OrderedMap {
	m := NewOrderedMap()
	m.Set("length", length)
	return m
}

func orderedPerformance(timing map[string]int64) *OrderedMap {
	m := NewOrderedMap()
	m.Set("timing", orderedPerfTiming(timing))
	return m
}

func orderedAutomation() *OrderedMap {
	wdProps := NewOrderedMap()
	wdProps.Set("document", []string{})
	wdProps.Set("window", []string{})
	wdProps.Set("navigator", []string{})

	wd := NewOrderedMap()
	wd.Set("properties", wdProps)

	phantomProps := NewOrderedMap()
	phantomProps.Set("window", []string{})

	phantom := NewOrderedMap()
	phantom.Set("properties", phantomProps)

	automation := NewOrderedMap()
	automation.Set("wd", wd)
	automation.Set("phantom", phantom)
	return automation
}

func orderedCapabilities(elapsed int) *OrderedMap {
	css := NewOrderedMap()
	css.Set("textShadow", 1)
	css.Set("WebkitTextStroke", 1)
	css.Set("boxShadow", 1)
	css.Set("borderRadius", 1)
	css.Set("borderImage", 1)
	css.Set("opacity", 1)
	css.Set("transform", 1)
	css.Set("transition", 1)

	js := NewOrderedMap()
	js.Set("audio", true)
	js.Set("geolocation", true)
	js.Set("localStorage", "supported")
	js.Set("touch", false)
	js.Set("video", true)
	js.Set("webWorker", true)

	capabilities := NewOrderedMap()
	capabilities.Set("css", css)
	capabilities.Set("js", js)
	capabilities.Set("elapsed", elapsed)
	return capabilities
}

func orderedGPU(identity *BrowserIdentity) *OrderedMap {
	m := NewOrderedMap()
	m.Set("vendor", identity.GPUVendor)
	m.Set("model", identity.GPUModel)
	m.Set("extensions", identity.WebGLExts)
	return m
}

func orderedMath(identity *BrowserIdentity) *OrderedMap {
	m := NewOrderedMap()
	m.Set("tan", identity.MathTan)
	m.Set("sin", identity.MathSin)
	m.Set("cos", identity.MathCos)
	return m
}

func orderedCanvas(hash int32, emailHash interface{}, histogramBins []int) *OrderedMap {
	m := NewOrderedMap()
	m.Set("hash", hash)
	m.Set("emailHash", emailHash)
	m.Set("histogramBins", histogramBins)
	return m
}

func orderedToken(isCompatible bool) *OrderedMap {
	m := NewOrderedMap()
	m.Set("isCompatible", isCompatible)
	m.Set("pageHasCaptcha", 0)
	return m
}

func orderedAuth(method string) *OrderedMap {
	form := NewOrderedMap()
	form.Set("method", method)

	auth := NewOrderedMap()
	auth.Set("form", form)
	return auth
}

func emptyOrderedMap() *OrderedMap {
	return NewOrderedMap()
}

// OrderedMap 有序 map，用于保证 JSON 字段顺序
type OrderedMap struct {
	keys   []string
	values map[string]interface{}
}

// NewOrderedMap 创建有序 map
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{values: make(map[string]interface{})}
}

// Set 设置键值对
func (o *OrderedMap) Set(key string, value interface{}) {
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *OrderedMap) Get(key string) (interface{}, bool) {
	if o == nil {
		return nil, false
	}
	value, ok := o.values[key]
	return value, ok
}

// MarshalJSON 序列化为有序 JSON
func (o *OrderedMap) MarshalJSON() ([]byte, error) {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		sb.Write(kb)
		sb.WriteByte(':')
		vb, _ := json.Marshal(o.values[k])
		sb.Write(vb)
	}
	sb.WriteByte('}')
	return []byte(sb.String()), nil
}

// MarshalOrdered 将有序 map 序列化为紧凑 JSON
func MarshalOrdered(m *OrderedMap) string {
	b, _ := m.MarshalJSON()
	return string(b)
}

// BuildFingerprintData 构建完整的指纹 JSON 数据
func BuildFingerprintData(
	identity *BrowserIdentity,
	locationURL, referrer string,
	nowMs int64,
	ctx *FingerprintContext,
	pageType, eventType string,
	timeOnPage, emailLen int,
	email string,
	timeZone ...int,
) *OrderedMap {
	tz := 8
	if len(timeZone) > 0 {
		tz = timeZone[0]
	}
	// 硬件级字段
	canvasHash := identity.CanvasHash
	histogram := identity.HistogramBase
	if ctx != nil {
		canvasHash = ctx.CanvasHash
		histogram = ctx.HistogramBins
	}

	// performance.timing
	var perfTiming map[string]int64
	if ctx != nil {
		perfTiming = ctx.GetPerfTiming(nowMs)
	} else {
		perfTiming = GenPerfTiming(nowMs)
	}
	if pageType == "profile" {
		applyProfileBrowserLoadTimingShape(perfTiming)
		applyPreviousDocumentUnloadTiming(perfTiming)
	}

	// lsUbid
	var lsUbid string
	if ctx != nil {
		lsUbid = ctx.GetLsUbid(pageType)
	} else {
		lsUbid = fmt.Sprintf("%s-%07d-%07d:%d",
			identity.LsubidPrefixSignin, rand.Intn(10000000), rand.Intn(10000000), perfTiming["loadEventEnd"]/1000)
	}

	// 页面相关字段
	var dynamicURLs []string
	var scriptsElapsed int
	var historyLength int
	var isCompatible bool

	switch pageType {
	case "profile":
		dynamicURLs = []string{fmt.Sprintf("/dist/main/app_%s.min.js", identity.WebpackHash)}
		scriptsElapsed = 0
		if eventType == "PageLoad" || eventType == "first_load" {
			historyLength = 2
		} else {
			historyLength = 4
		}
		isCompatible = eventType == "PageLoad" || eventType == "first_load"
	case "signup":
		dynamicURLs = []string{"/assets/js/app.js"}
		scriptsElapsed = 1
		historyLength = 5
		isCompatible = true
	default:
		dynamicURLs = []string{"/assets/js/app.js"}
		scriptsElapsed = 1
		historyLength = 1
		isCompatible = false
	}

	// metrics
	var metrics interface{}
	if eventType == "first_load" || (eventType == "PageLoad" && pageType == "profile") {
		metrics = orderedMetrics(genMetricsFirstLoad(pageType))
	} else {
		metrics = genMetricsPageSubmit()
	}
	var inlineHashes interface{} = []string{}
	inlineHashesCount := 0
	if pageType == "profile" {
		if scriptInfo, ok := ctx.ProfileScriptData(); ok {
			dynamicURLs = append([]string(nil), scriptInfo.DynamicURLs...)
			inlineHashes = append([]uint32(nil), scriptInfo.InlineHashes...)
			inlineHashesCount = len(scriptInfo.InlineHashes)
			scriptsElapsed = 0
		}
	}

	// interaction
	interaction := genInteraction(pageType, eventType)

	// start / end 时间
	endMs := nowMs + int64(rand.Intn(51))
	var startTime int64
	if eventType != "PageLoad" && eventType != "first_load" && timeOnPage > 0 {
		startTime = endMs - int64(timeOnPage)
	} else if ctx != nil {
		if eventType == "first_load" {
			startTime = ctx.GetStartTime(nowMs - int64(500+rand.Intn(501)))
		} else if eventType == "PageLoad" && pageType == "profile" {
			startTime = ctx.GetStartTime(nowMs - int64(30+rand.Intn(51)))
		} else {
			startTime = ctx.GetStartTime(nowMs)
		}
	} else {
		startTime = nowMs
	}

	pluginsStr := formatPlugins(identity.Plugins)
	screenStr := formatScreen(identity.Screen)

	// 组装 (字段顺序严格按真实抓包)
	result := NewOrderedMap()
	result.Set("metrics", metrics)
	result.Set("start", startTime)
	result.Set("interaction", interaction)
	result.Set("scripts", orderedScripts(dynamicURLs, inlineHashes, scriptsElapsed, inlineHashesCount))
	result.Set("history", orderedHistory(historyLength))
	result.Set("battery", emptyOrderedMap())
	result.Set("performance", orderedPerformance(perfTiming))
	result.Set("automation", orderedAutomation())
	result.Set("end", endMs)
	result.Set("timeZone", tz)
	result.Set("flashVersion", nil)
	result.Set("plugins", pluginsStr+"||"+screenStr)
	result.Set("dupedPlugins", pluginsStr+"||"+screenStr)
	result.Set("screenInfo", screenStr)
	result.Set("lsUbid", lsUbid)
	result.Set("referrer", referrer)
	result.Set("userAgent", identity.UA)
	if pageType != "profile" {
		result.Set("deviceMemory", identity.DeviceMemory)
		result.Set("hardwareConcurrency", identity.HardwareConcurrency)
		result.Set("platform", identity.Platform)
	}
	result.Set("location", locationURL)
	result.Set("webDriver", false)
	capabilitiesElapsed := 0
	if pageType == "profile" && eventType != "PageLoad" && eventType != "first_load" {
		capabilitiesElapsed = 1
	}
	result.Set("capabilities", orderedCapabilities(capabilitiesElapsed))
	result.Set("gpu", orderedGPU(identity))
	result.Set("dnt", nil)
	result.Set("math", orderedMath(identity))

	// profile 页面的 timeToSubmit
	if pageType == "profile" {
		if eventType == "PageLoad" || eventType == "first_load" {
			result.Set("timeToSubmit", 1+rand.Intn(5))
		} else if timeOnPage > 0 {
			result.Set("timeToSubmit", timeOnPage)
		} else {
			result.Set("timeToSubmit", 2000+rand.Intn(4001))
		}
	}

	// form 字段
	if pageType == "profile" && eventType != "PageLoad" && eventType != "first_load" && emailLen > 0 {
		result.Set("form", genFormField(nowMs, emailLen, email, interaction))
	} else {
		result.Set("form", emptyOrderedMap())
	}

	// canvas
	histSlice := make([]int, 256)
	copy(histSlice, histogram[:])
	var canvasEmailHash interface{} = nil
	if strings.TrimSpace(email) != "" {
		// 真实 profile FWCIM collector 在已 profile 的表单上复用同一张 canvas；
		// 本地 Chromium 明文捕获显示不同邮箱提交时 emailHash 保持同一稳定 CRC。
		canvasEmailHash = uint32(60428351)
	}
	result.Set("canvas", orderedCanvas(canvasHash, canvasEmailHash, histSlice))
	result.Set("token", orderedToken(isCompatible))
	authMethod := "get"
	if pageType == "profile" && eventType != "PageLoad" && eventType != "first_load" {
		authMethod = "post"
	}
	result.Set("auth", orderedAuth(authMethod))
	result.Set("errors", []interface{}{})
	result.Set("version", crypto.GetTESVersion())

	return result
}
