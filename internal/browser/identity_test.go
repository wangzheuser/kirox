package browser

import (
	"strings"
	"testing"
)

func TestGeneratedChromeVersionUsesTLSProfileSupportedMajor(t *testing.T) {
	supported := map[string]bool{"120": true, "124": true, "131": true, "133": true, "144": true}
	for i := 0; i < 200; i++ {
		cv := genChromeVersion()
		major := cv.Version
		if dot := strings.IndexByte(major, '.'); dot >= 0 {
			major = major[:dot]
		}
		if !supported[major] {
			t.Fatalf("generated Chrome major %s has no matching tls-client profile (version=%s)", major, cv.Version)
		}
	}
}

func TestGeneratedMathFingerprintUsesChromeLikeBaselineValues(t *testing.T) {
	for i := 0; i < 50; i++ {
		tan, sin, cos := genMath()
		if !strings.HasPrefix(tan, "-1.421448823874724") {
			t.Fatalf("tan=%q, want Chrome-like -1.421448823874724x", tan)
		}
		if !strings.HasPrefix(sin, "0.817881912115908") {
			t.Fatalf("sin=%q, want Chrome-like 0.817881912115908x", sin)
		}
		if !strings.HasPrefix(cos, "-0.57538611195754") && !strings.HasPrefix(cos, "-0.57657750042868") {
			t.Fatalf("cos=%q, want known Chrome-like cos family", cos)
		}
	}
}

func TestRandomIdentityUsesConservativeNavigatorHardwareSurface(t *testing.T) {
	for i := 0; i < 500; i++ {
		identity := RandomIdentity()
		if identity.DeviceMemory < 4 || identity.DeviceMemory > 8 {
			t.Fatalf("deviceMemory=%dGB, want conservative Chrome bucket 4-8GB", identity.DeviceMemory)
		}
		if identity.HardwareConcurrency < 4 || identity.HardwareConcurrency > 16 {
			t.Fatalf("hardwareConcurrency=%d, want common desktop/laptop range 4-16", identity.HardwareConcurrency)
		}
		if identity.Screen.ColorDepth != 24 {
			t.Fatalf("colorDepth=%d, want common non-HDR Chrome colorDepth 24", identity.Screen.ColorDepth)
		}
		if identity.Screen.Width > 2560 || identity.Screen.Height > 1440 {
			t.Fatalf("screen=%dx%d, want conservative <=2560x1440", identity.Screen.Width, identity.Screen.Height)
		}
		if identity.Screen.Width < 1366 || identity.Screen.Height < 768 {
			t.Fatalf("screen=%dx%d, want common desktop/laptop minimum 1366x768", identity.Screen.Width, identity.Screen.Height)
		}
	}
}

func TestCachedIdentityRejectsHighRiskHardwareSurface(t *testing.T) {
	identity := RandomIdentity()
	identity.DeviceMemory = 64
	identity.HardwareConcurrency = 32
	identity.Screen.ColorDepth = 30
	identity.Screen.Width = 3840
	identity.Screen.Height = 2160

	if isConservativeBrowserIdentity(identity) {
		t.Fatalf("cached high-risk identity should be rejected and regenerated")
	}
}
