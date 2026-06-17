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
