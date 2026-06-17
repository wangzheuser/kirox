package browser

import (
	"strings"
	"testing"
)

func TestFormatPluginsKeepsChromePluginStrTrailingSpace(t *testing.T) {
	plugins := []map[string]string{
		{"name": "PDF Viewer", "description": "Portable Document Format"},
		{"name": "Chrome PDF Viewer", "description": "Portable Document Format"},
	}

	got := formatPlugins(plugins)
	want := "PDF Viewer Chrome PDF Viewer "
	if got != want {
		t.Fatalf("formatPlugins()=%q, want %q", got, want)
	}
	if !strings.HasSuffix(got, " ") {
		t.Fatalf("formatPlugins()=%q, want trailing space before ||screen", got)
	}
}

func TestFingerprintPluginsUseSingleSpaceBeforeScreenInfoSeparator(t *testing.T) {
	identity := RandomIdentity()
	identity.Plugins = []map[string]string{
		{"name": "PDF Viewer", "description": "Portable Document Format"},
		{"name": "Chrome PDF Viewer", "description": "Portable Document Format"},
	}
	identity.Screen = ScreenInfo{Width: 1920, Height: 1080, AvailHeight: 1040, ColorDepth: 24}

	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, NewFPContext(identity), "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	want := `"plugins":"PDF Viewer Chrome PDF Viewer ||1920-1080-1040-24-*-*-*"`
	if !strings.Contains(raw, want) {
		t.Fatalf("fingerprint plugins JSON does not contain %s; raw=%s", want, raw)
	}
	if strings.Contains(raw, "Chrome PDF Viewer  ||") {
		t.Fatalf("fingerprint plugins contains double space before separator: %s", raw)
	}
}
