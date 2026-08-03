package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"reg_go/internal/storage"
)

func TestMain(m *testing.M) {
	tempRoot, err := os.MkdirTemp("", "kirox-task-tests-*")
	if err != nil {
		panic(err)
	}
	for _, key := range []string{"APPDATA", "XDG_CONFIG_HOME", "HOME", "USERPROFILE"} {
		_ = os.Setenv(key, tempRoot)
	}
	dataDir := filepath.Join(tempRoot, "data")
	resultDir := filepath.Join(tempRoot, "results")
	_, dataErr := storage.SetDataDirPath(dataDir)
	_, resultErr := storage.SetResultOutputDir(resultDir)
	if dataErr != nil || resultErr != nil {
		_ = os.RemoveAll(tempRoot)
		panic("failed to isolate task test storage")
	}

	code := m.Run()
	_ = Shutdown(5 * time.Second)
	_ = os.RemoveAll(tempRoot)
	os.Exit(code)
}
