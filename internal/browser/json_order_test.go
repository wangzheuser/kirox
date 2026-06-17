package browser

import (
	"strings"
	"testing"
)

func fixedProfileSubmitJSONForOrderTest() string {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	ctx.SetProfileHTML(`<html><head><script src="/dist/main/app_realhash.min.js"></script><script>window.__profileBoot=true;</script></head></html>`)
	return MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", 6123, len("user@example.test"), "user@example.test", -8))
}

func TestFingerprintMetricsJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()

	want := `"metrics":{"el":0,"script":0,"h":0,"batt":0,"perf":`
	if !strings.Contains(raw, want) {
		t.Fatalf("metrics JSON does not start in collector order %s; raw prefix=%s", want, raw[:min(300, len(raw))])
	}
}

func TestFingerprintInteractionJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()

	idx := strings.Index(raw, `"interaction":`)
	if idx < 0 {
		t.Fatalf("interaction JSON missing; raw=%s", raw)
	}
	segment := raw[idx:min(len(raw), idx+900)]
	assertJSONOrder(t, segment, []string{`"clicks":`, `"touches":`, `"keyPresses":`, `"cuts":`, `"copies":`, `"pastes":`, `"keyPressTimeIntervals":`, `"mouseClickPositions":`, `"keyCycles":`, `"mouseCycles":`, `"touchCycles":`})
}

func TestFingerprintScriptsJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()
	idx := strings.Index(raw, `"scripts":`)
	if idx < 0 {
		t.Fatalf("scripts JSON missing; raw=%s", raw)
	}
	segment := raw[idx:min(len(raw), idx+220)]
	assertJSONOrder(t, segment, []string{`"dynamicUrls":`, `"inlineHashes":`, `"elapsed":`, `"dynamicUrlCount":`, `"inlineHashesCount":`})
}

func TestFingerprintPerformanceTimingJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()
	idx := strings.Index(raw, `"timing":`)
	if idx < 0 {
		t.Fatalf("performance.timing JSON missing; raw=%s", raw)
	}
	segment := raw[idx:min(len(raw), idx+900)]
	assertJSONOrder(t, segment, []string{`"connectStart":`, `"secureConnectionStart":`, `"unloadEventEnd":`, `"domainLookupStart":`, `"domainLookupEnd":`, `"responseStart":`, `"connectEnd":`, `"responseEnd":`, `"requestStart":`, `"domLoading":`, `"redirectStart":`, `"loadEventEnd":`, `"domComplete":`, `"navigationStart":`, `"loadEventStart":`, `"domContentLoadedEventEnd":`, `"unloadEventStart":`, `"redirectEnd":`, `"domInteractive":`, `"fetchStart":`, `"domContentLoadedEventStart":`})
}

func TestFingerprintAutomationJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()
	idx := strings.Index(raw, `"automation":`)
	if idx < 0 {
		t.Fatalf("automation JSON missing; raw=%s", raw)
	}
	segment := raw[idx:min(len(raw), idx+180)]
	assertJSONOrder(t, segment, []string{`"wd":`, `"document":`, `"window":`, `"navigator":`, `"phantom":`})
}

func TestFingerprintCapabilitiesJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()
	anchor := `"webDriver":false,`
	anchorIdx := strings.Index(raw, anchor)
	if anchorIdx < 0 {
		t.Fatalf("webDriver anchor missing; raw=%s", raw)
	}
	idx := anchorIdx + strings.Index(raw[anchorIdx:], `"capabilities":`)
	if idx < anchorIdx {
		t.Fatalf("capabilities JSON missing after webDriver; raw=%s", raw)
	}
	segment := raw[idx:min(len(raw), idx+900)]
	assertJSONOrder(t, segment, []string{`"css":`, `"textShadow":`, `"WebkitTextStroke":`, `"boxShadow":`, `"borderRadius":`, `"borderImage":`, `"opacity":`, `"transform":`, `"transition":`, `"js":`, `"audio":`, `"geolocation":`, `"localStorage":`, `"touch":`, `"video":`, `"webWorker":`, `"elapsed":`})
}

func TestFingerprintGPUJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()
	idx := strings.Index(raw, `"gpu":{`)
	if idx < 0 {
		t.Fatalf("gpu JSON missing; raw=%s", raw)
	}
	segment := raw[idx:min(len(raw), idx+220)]
	assertJSONOrder(t, segment, []string{`"vendor":`, `"model":`, `"extensions":`})
}

func TestFingerprintMathCanvasTokenAuthJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()

	idx := strings.Index(raw, `"math":{`)
	if idx < 0 {
		t.Fatalf("math JSON missing; raw=%s", raw)
	}
	assertJSONOrder(t, raw[idx:min(len(raw), idx+120)], []string{`"tan":`, `"sin":`, `"cos":`})

	idx = strings.Index(raw, `"canvas":{`)
	if idx < 0 {
		t.Fatalf("canvas JSON missing; raw=%s", raw)
	}
	assertJSONOrder(t, raw[idx:min(len(raw), idx+180)], []string{`"hash":`, `"emailHash":`, `"histogramBins":`})

	idx = strings.Index(raw, `"token":`)
	if idx < 0 {
		t.Fatalf("token JSON missing; raw=%s", raw)
	}
	assertJSONOrder(t, raw[idx:min(len(raw), idx+120)], []string{`"isCompatible":`, `"pageHasCaptcha":`})

	idx = strings.Index(raw, `"auth":`)
	if idx < 0 {
		t.Fatalf("auth JSON missing; raw=%s", raw)
	}
	assertJSONOrder(t, raw[idx:min(len(raw), idx+100)], []string{`"form":`, `"method":`})
}

func TestFingerprintFormFieldJSONUsesCollectorOrder(t *testing.T) {
	raw := fixedProfileSubmitJSONForOrderTest()
	idx := strings.Index(raw, `"form":{"email":`)
	if idx < 0 {
		t.Fatalf("form email JSON missing; raw=%s", raw)
	}
	segment := raw[idx:min(len(raw), idx+900)]
	assertJSONOrder(t, segment, []string{`"clicks":`, `"touches":`, `"keyPresses":`, `"cuts":`, `"copies":`, `"pastes":`, `"keyPressTimeIntervals":`, `"mouseClickPositions":`, `"keyCycles":`, `"mouseCycles":`, `"touchCycles":`, `"width":`, `"height":`, `"totalFocusTime":`, `"checksum":`, `"autocomplete":`, `"prefilled":`})
}

func assertJSONOrder(t *testing.T, raw string, orderedKeys []string) {
	t.Helper()
	last := -1
	for _, key := range orderedKeys {
		idx := strings.Index(raw, key)
		if idx < 0 {
			t.Fatalf("%s missing from JSON segment: %s", key, raw)
		}
		if idx <= last {
			t.Fatalf("%s appears out of order in JSON segment: %s", key, raw)
		}
		last = idx
	}
}
