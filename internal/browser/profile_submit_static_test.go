package browser

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProfileSubmitStaticCollectorFieldsMatchFWCIMCapture(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/?workflowID=wf#/signup/enter-email", "https://signin.aws.amazon.com/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	history := decoded["history"].(map[string]interface{})
	if got := history["length"]; got != float64(4) {
		t.Fatalf("history.length=%v, want captured profile submit length 4", got)
	}
	capabilities := decoded["capabilities"].(map[string]interface{})
	if got := capabilities["elapsed"]; got != float64(1) {
		t.Fatalf("capabilities.elapsed=%v, want captured profile submit elapsed 1", got)
	}
}

func TestProfileSubmitCanvasHistogramMatchesFWCIMCanvasShape(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/?workflowID=wf#/signup/enter-email", "https://signin.aws.amazon.com/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	canvas := decoded["canvas"].(map[string]interface{})
	binsRaw := canvas["histogramBins"].([]interface{})
	bins := make([]int, len(binsRaw))
	total := 0
	for i, rawBin := range binsRaw {
		bins[i] = int(rawBin.(float64))
		total += bins[i]
	}
	if len(bins) != 256 {
		t.Fatalf("histogramBins len=%d, want 256", len(bins))
	}
	if total != 36000 {
		t.Fatalf("histogram total=%d, want 36000 RGBA samples from 150x60 canvas", total)
	}
	if bins[0] < 12000 || bins[0] > 14000 || bins[255] < 12000 || bins[255] > 14000 {
		t.Fatalf("canvas background/alpha peaks bins[0]=%d bins[255]=%d, want Chrome FWCIM profile shape around 12.8k", bins[0], bins[255])
	}
	if bins[102] < 450 || bins[102] > 650 {
		t.Fatalf("canvas #f60 green-channel spike bins[102]=%d, want captured FWCIM range 450..650", bins[102])
	}
}

func TestProfileSubmitCanvasEmailHashMatchesFWCIMCaptureForEmail(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/?workflowID=wf#/signup/enter-email", "https://signin.aws.amazon.com/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	canvas := decoded["canvas"].(map[string]interface{})
	if got := canvas["hash"]; got != float64(-2120415875) {
		t.Fatalf("canvas.hash=%v, want captured FWCIM base hash", got)
	}
	if got := canvas["emailHash"]; got != float64(60428351) {
		t.Fatalf("canvas.emailHash=%v, want captured FWCIM email canvas CRC 60428351 for user@example.test", got)
	}
}

func TestProfileSubmitPerformanceTimingIncludesPreviousDocumentUnload(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/?workflowID=wf#/signup/enter-email", "https://signin.aws.amazon.com/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	timing := decoded["performance"].(map[string]interface{})["timing"].(map[string]interface{})
	navigationStart := timing["navigationStart"].(float64)
	unloadStart := timing["unloadEventStart"].(float64)
	unloadEnd := timing["unloadEventEnd"].(float64)
	responseEnd := timing["responseEnd"].(float64)

	if unloadStart == 0 || unloadEnd == 0 {
		t.Fatalf("unload timing start/end=%v/%v, want non-zero previous-document unload events like captured Chromium profile submit", unloadStart, unloadEnd)
	}
	if unloadStart != unloadEnd {
		t.Fatalf("unload timing start/end=%v/%v, want same-millisecond unload event from captured Chromium profile submit", unloadStart, unloadEnd)
	}
	if unloadStart < navigationStart || unloadEnd > responseEnd {
		t.Fatalf("unload timing start/end=%v/%v outside navigationStart..responseEnd %v..%v", unloadStart, unloadEnd, navigationStart, responseEnd)
	}
}

func TestProfileSubmitPerformanceTimingUsesShortBrowserLoadShape(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/?workflowID=wf#/signup/enter-email", "https://signin.aws.amazon.com/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	timing := decoded["performance"].(map[string]interface{})["timing"].(map[string]interface{})
	navigationStart := timing["navigationStart"].(float64)
	responseEndOffset := timing["responseEnd"].(float64) - navigationStart
	domInteractiveOffset := timing["domInteractive"].(float64) - navigationStart
	loadEndOffset := timing["loadEventEnd"].(float64) - navigationStart

	if responseEndOffset < 15 || responseEndOffset > 80 {
		t.Fatalf("responseEnd offset=%vms, want short captured browser navigation shape 15..80ms", responseEndOffset)
	}
	if domInteractiveOffset < 70 || domInteractiveOffset > 180 {
		t.Fatalf("domInteractive offset=%vms, want captured browser DOM-ready shape 70..180ms", domInteractiveOffset)
	}
	if loadEndOffset < 80 || loadEndOffset > 220 {
		t.Fatalf("loadEventEnd offset=%vms, want captured browser load shape 80..220ms", loadEndOffset)
	}
}

func TestProfilePageLoadAndSubmitReuseSamePerformanceTiming(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	pageLoadRaw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/?workflowID=wf#/signup/start", "https://signin.aws.amazon.com/", 1781600000000, ctx, "profile", "PageLoad", 0, "", -8))
	pageSubmitRaw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/?workflowID=wf#/signup/enter-email", "https://signin.aws.amazon.com/", 1781600007000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var pageLoad map[string]interface{}
	if err := json.Unmarshal([]byte(pageLoadRaw), &pageLoad); err != nil {
		t.Fatalf("PageLoad fingerprint JSON decode failed: %v", err)
	}
	var pageSubmit map[string]interface{}
	if err := json.Unmarshal([]byte(pageSubmitRaw), &pageSubmit); err != nil {
		t.Fatalf("PageSubmit fingerprint JSON decode failed: %v", err)
	}
	loadTiming := pageLoad["performance"].(map[string]interface{})["timing"].(map[string]interface{})
	submitTiming := pageSubmit["performance"].(map[string]interface{})["timing"].(map[string]interface{})
	if !reflect.DeepEqual(loadTiming, submitTiming) {
		t.Fatalf("profile PageLoad and PageSubmit timing differ on the same document\nPageLoad=%#v\nPageSubmit=%#v", loadTiming, submitTiming)
	}
}
