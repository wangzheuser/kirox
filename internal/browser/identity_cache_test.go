package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"reg_go/internal/storage"
)

func isolateIdentityCacheStorage(t *testing.T) {
	t.Helper()
	tempRoot := t.TempDir()
	t.Setenv("APPDATA", tempRoot)
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)
	t.Setenv("USERPROFILE", tempRoot)
	if _, err := storage.SetDataDirPath(tempRoot); err != nil {
		t.Fatalf("SetDataDirPath: %v", err)
	}
	idCacheMu.Lock()
	idCache = nil
	idCacheMu.Unlock()
	t.Cleanup(func() {
		idCacheMu.Lock()
		idCache = nil
		idCacheMu.Unlock()
	})
}

func writeIdentityCacheForTest(t *testing.T, entries map[string]cachedIdentity) {
	t.Helper()
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal identity cache: %v", err)
	}
	if err := os.WriteFile(identityCachePath(), b, 0o600); err != nil {
		t.Fatalf("write identity cache: %v", err)
	}
}

func readIdentityCacheForTest(t *testing.T) map[string]cachedIdentity {
	t.Helper()
	b, err := os.ReadFile(identityCachePath())
	if err != nil {
		t.Fatalf("read identity cache: %v", err)
	}
	var entries map[string]cachedIdentity
	if err := json.Unmarshal(b, &entries); err != nil {
		t.Fatalf("unmarshal identity cache: %v", err)
	}
	return entries
}

func TestIdentityForProxyPrunesExpiredAndInvalidCachedIdentities(t *testing.T) {
	isolateIdentityCacheStorage(t)

	now := time.Now().Unix()
	fresh := RandomIdentity()
	invalid := RandomIdentity()
	invalid.DeviceMemory = 64
	writeIdentityCacheForTest(t, map[string]cachedIdentity{
		"fresh.example:8080": {
			Identity:  fresh,
			CreatedAt: now - int64((identityCacheTTL / 2).Seconds()),
		},
		"expired.example:8080": {
			Identity:  RandomIdentity(),
			CreatedAt: now - int64((identityCacheTTL + time.Hour).Seconds()),
		},
		"invalid.example:8080": {
			Identity:  invalid,
			CreatedAt: now,
		},
	})

	IdentityForProxy("http://new.example:8080")

	entries := readIdentityCacheForTest(t)
	if _, ok := entries["fresh.example:8080"]; !ok {
		t.Fatalf("fresh identity should be preserved: %#v", entries)
	}
	if _, ok := entries["expired.example:8080"]; ok {
		t.Fatalf("expired identity should be pruned: %#v", entries)
	}
	if _, ok := entries["invalid.example:8080"]; ok {
		t.Fatalf("invalid identity should be pruned: %#v", entries)
	}
	if _, ok := entries["new.example:8080"]; !ok {
		t.Fatalf("new identity should be persisted: %#v", entries)
	}
}

func TestIdentityForProxyCapsPersistedCacheEntries(t *testing.T) {
	isolateIdentityCacheStorage(t)

	now := time.Now().Unix()
	entries := make(map[string]cachedIdentity, identityCacheMaxEntries+20)
	for i := 0; i < identityCacheMaxEntries+20; i++ {
		entries[fmt.Sprintf("proxy-%04d.example:8080", i)] = cachedIdentity{
			Identity:  RandomIdentity(),
			CreatedAt: now - int64(identityCacheMaxEntries+20-i),
		}
	}
	writeIdentityCacheForTest(t, entries)

	IdentityForProxy("http://new.example:8080")

	got := readIdentityCacheForTest(t)
	if len(got) > identityCacheMaxEntries {
		t.Fatalf("identity cache should be capped at %d entries, got %d", identityCacheMaxEntries, len(got))
	}
	if _, ok := got["proxy-0000.example:8080"]; ok {
		t.Fatalf("oldest identity should be pruned when cache is capped")
	}
	if _, ok := got["new.example:8080"]; !ok {
		t.Fatalf("new identity should be preserved after capping")
	}
}
