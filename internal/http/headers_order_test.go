package http

import (
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

func TestSetHeadersUsesStableBrowserLikeOrder(t *testing.T) {
	req, err := fhttp.NewRequest("POST", "https://example.test/api", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	SetHeaders(req, map[string]string{
		"User-Agent":         "ua",
		"Accept":             "*/*",
		"Content-Type":       "application/json",
		"Origin":             "https://example.test",
		"Referer":            "https://example.test/",
		"sec-ch-ua":          `"Chromium";v="124"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"priority":           "u=1, i",
		"Accept-Language":    "en-US,en;q=0.9",
		"Accept-Encoding":    "gzip, deflate, br",
	})

	got := req.Header[fhttp.HeaderOrderKey]
	want := []string{"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "upgrade-insecure-requests", "user-agent", "accept", "content-type", "origin", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest", "referer", "accept-encoding", "accept-language", "cookie", "priority"}
	if len(got) != 14 {
		t.Fatalf("HeaderOrder length=%d, got %v", len(got), got)
	}
	last := -1
	for _, key := range got {
		pos := -1
		for i, candidate := range want {
			if candidate == key {
				pos = i
				break
			}
		}
		if pos < 0 {
			t.Fatalf("unexpected header %q in order %v", key, got)
		}
		if pos < last {
			t.Fatalf("header order is not browser-like/stable: %v", got)
		}
		last = pos
	}
}
