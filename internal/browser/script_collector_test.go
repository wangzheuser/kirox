package browser

import (
	"encoding/json"
	"hash/crc32"
	"testing"
)

func TestFingerprintContextParsesProfileScriptsFromHTML(t *testing.T) {
	identity := RandomIdentity()
	ctx := NewFPContext(identity)
	ctx.SetProfileHTML(`<html><head>` +
		`<script src="/dist/main/app_realhash.min.js"></script>` +
		`<script>window.__profileBoot = true;</script>` +
		`</head></html>`)

	raw := MarshalOrdered(BuildFingerprintData(identity, "https://profile.aws.amazon.com/", "https://example.test/", 1781600000000, ctx, "profile", "PageSubmit", len("user@example.test"), "user@example.test", -8))

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("fingerprint JSON decode failed: %v", err)
	}
	scripts, ok := decoded["scripts"].(map[string]interface{})
	if !ok {
		t.Fatalf("scripts has type %T, want object", decoded["scripts"])
	}
	dynamicURLs, ok := scripts["dynamicUrls"].([]interface{})
	if !ok {
		t.Fatalf("dynamicUrls has type %T, want array", scripts["dynamicUrls"])
	}
	if len(dynamicURLs) != 1 || dynamicURLs[0] != "/dist/main/app_realhash.min.js" {
		t.Fatalf("dynamicUrls=%v, want real profile script URL", dynamicURLs)
	}
	inlineHashes, ok := scripts["inlineHashes"].([]interface{})
	if !ok {
		t.Fatalf("inlineHashes has type %T, want array", scripts["inlineHashes"])
	}
	wantHash := float64(crc32.ChecksumIEEE([]byte(`<script>window.__profileBoot = true;</script>`)))
	if len(inlineHashes) != 1 || inlineHashes[0] != wantHash {
		t.Fatalf("inlineHashes=%v, want [%v]", inlineHashes, wantHash)
	}
	if scripts["dynamicUrlCount"] != float64(1) || scripts["inlineHashesCount"] != float64(1) {
		t.Fatalf("script counts = dynamic %v inline %v, want 1/1", scripts["dynamicUrlCount"], scripts["inlineHashesCount"])
	}
}
