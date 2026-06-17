package browser

import (
	"fmt"
	"hash/crc32"
	"math/rand"
	"regexp"
	"time"
)

// FingerprintContext 保持同一会话内硬件级指纹字段不变
type FingerprintContext struct {
	Identity       *BrowserIdentity
	CanvasHash     int32
	HistogramBins  [256]int
	LsUbidSignin   string
	LsUbidProfile  string
	ProfileScripts ProfileScriptInfo
	perfTiming     map[string]int64
	startTime      *int64
}

type ProfileScriptInfo struct {
	DynamicURLs  []string
	InlineHashes []uint32
}

// NewFPContext 创建指纹上下文
func NewFPContext(identity *BrowserIdentity) *FingerprintContext {
	ts := time.Now().Unix()
	return &FingerprintContext{
		Identity:      identity,
		CanvasHash:    identity.CanvasHash,
		HistogramBins: identity.HistogramBase,
		LsUbidSignin: fmt.Sprintf("%s-%07d-%07d:%d",
			identity.LsubidPrefixSignin, rand.Intn(10000000), rand.Intn(10000000), ts),
	}
}

// GetLsUbid 获取对应域名的 lsUbid
func (c *FingerprintContext) GetLsUbid(pageType string) string {
	if pageType == "profile" {
		if c.LsUbidProfile == "" {
			var ts int64
			if c.perfTiming != nil {
				ts = c.perfTiming["loadEventEnd"] / 1000
			} else {
				ts = time.Now().Unix()
			}
			c.LsUbidProfile = fmt.Sprintf("%s-%07d-%07d:%d",
				c.Identity.LsubidPrefixProfile, rand.Intn(10000000), rand.Intn(10000000), ts)
		}
		return c.LsUbidProfile
	}
	return c.LsUbidSignin
}

// GetPerfTiming 获取 performance.timing
func (c *FingerprintContext) GetPerfTiming(nowMs int64) map[string]int64 {
	if c.perfTiming == nil {
		c.perfTiming = GenPerfTiming(nowMs)
	}
	return c.perfTiming
}

// GetStartTime 获取页面 start 时间戳
func (c *FingerprintContext) GetStartTime(nowMs int64) int64 {
	if c.startTime == nil {
		t := nowMs
		c.startTime = &t
	}
	return *c.startTime
}

// ResetPerfTiming 切换到新页面时重置 timing
func (c *FingerprintContext) ResetPerfTiming() {
	c.perfTiming = nil
}

var scriptTagRE = regexp.MustCompile(`(?is)<script[\s\S]*?>[\s\S]*?</script>`)
var scriptSrcRE = regexp.MustCompile(`(?is)src="[\s\S]*?"`)

func (c *FingerprintContext) SetProfileHTML(html string) {
	info := parseProfileScripts(html)
	c.ProfileScripts = info
}

func parseProfileScripts(html string) ProfileScriptInfo {
	var info ProfileScriptInfo
	for _, script := range scriptTagRE.FindAllString(html, -1) {
		if src := scriptSrcRE.FindString(script); src != "" {
			if len(src) > 6 {
				info.DynamicURLs = append(info.DynamicURLs, src[5:len(src)-1])
			}
			continue
		}
		info.InlineHashes = append(info.InlineHashes, crc32.ChecksumIEEE([]byte(script)))
	}
	return info
}

func (c *FingerprintContext) ProfileScriptData() (ProfileScriptInfo, bool) {
	if c == nil {
		return ProfileScriptInfo{}, false
	}
	return c.ProfileScripts, len(c.ProfileScripts.DynamicURLs) > 0 || len(c.ProfileScripts.InlineHashes) > 0
}

// GenerateFingerprintJSON 生成指纹 JSON（不加密），供远程加密使用
func GenerateFingerprintJSON(
	identity *BrowserIdentity,
	locationURL, referrer string,
	ctx *FingerprintContext,
	pageType, eventType string,
	timeOnPage, emailLen int,
	email string,
	timeZone ...int,
) string {
	nowMs := time.Now().UnixMilli()
	tz := 8
	if len(timeZone) > 0 {
		tz = timeZone[0]
	}
	fpData := BuildFingerprintData(identity, locationURL, referrer, nowMs, ctx,
		pageType, eventType, timeOnPage, emailLen, email, tz)
	return MarshalOrdered(fpData)
}
