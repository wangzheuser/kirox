package storage

import (
	"os"
	"strings"
	"testing"
)

func TestStorageRegistrationConfigTestsUseTempStorage(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	targetCall := "Set" + "RegistrationConfig("
	isolationCall := "with" + "TempStorageConfig(t,"
	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		source := string(data)
		if strings.Contains(source, targetCall) && !strings.Contains(source, isolationCall) {
			offenders = append(offenders, name)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("tests that call SetRegistrationConfig must isolate storage with withTempStorageConfig: %s", strings.Join(offenders, ", "))
	}
}
