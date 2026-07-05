package http

import (
	"testing"
	"time"
)

func TestChromeProfileKeyFromVersion(t *testing.T) {
	cases := map[string]string{
		"144.0.0.0": "chrome_144",
		"133.0.0.0": "chrome_133",
		"131.0.0.0": "chrome_131",
		"124.0.0.0": "chrome_124",
		"120.0.0.0": "chrome_120",
		"140.0.0.0": "chrome_144",
		"":          "chrome_144",
	}
	for input, want := range cases {
		if got := chromeProfileKeyFromVersion(input); got != want {
			t.Fatalf("chromeProfileKeyFromVersion(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestTLSClientTransportOptionsLimitIdleConnections(t *testing.T) {
	defaultOptions := defaultTLSClientTransportOptions()
	if defaultOptions.DisableKeepAlives {
		t.Fatalf("default registration client should keep intra-flow reuse available")
	}
	if defaultOptions.IdleConnTimeout == nil || *defaultOptions.IdleConnTimeout != 10*time.Second {
		t.Fatalf("default idle timeout = %v, want 10s", defaultOptions.IdleConnTimeout)
	}
	if defaultOptions.MaxIdleConns != 32 || defaultOptions.MaxIdleConnsPerHost != 4 {
		t.Fatalf("unexpected default idle limits: %#v", defaultOptions)
	}

	oneShotOptions := oneShotTLSClientTransportOptions()
	if !oneShotOptions.DisableKeepAlives {
		t.Fatalf("one-shot probe client should disable keep-alive")
	}
	if oneShotOptions.MaxIdleConnsPerHost != -1 {
		t.Fatalf("one-shot MaxIdleConnsPerHost=%d, want -1", oneShotOptions.MaxIdleConnsPerHost)
	}
}
