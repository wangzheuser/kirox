package email

import (
	"os"
	"path/filepath"
	"testing"

	"reg_go/internal/storage"
)

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "kirox-email-tests-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("APPDATA", filepath.Join(tmp, "appdata"))
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdg-config"))
	_ = os.Setenv("HOME", filepath.Join(tmp, "home"))

	code := m.Run()
	storage.FlushAccountsSync()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}
