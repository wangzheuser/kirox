package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitLogFileTruncatesTrailingNUL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, append([]byte("valid\n"), make([]byte, 128)...), 0o600); err != nil {
		t.Fatal(err)
	}
	state := &State{}
	if err := state.InitLogFile(dir); err != nil {
		t.Fatal(err)
	}
	state.AppendLog("next\n")
	if err := state.CloseLogFile(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "valid\nnext\n" {
		t.Fatalf("got %q", got)
	}
}

func TestInitLogFileRedactsCurrentAndRotatedLogs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"app.log", "app-2026-08-03T12-00-00.000.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("email=user@example.com user_code=ABCD-EFGH\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	state := &State{}
	if err := state.InitLogFile(dir); err != nil {
		t.Fatal(err)
	}
	if err := state.CloseLogFile(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app.log", "app-2026-08-03T12-00-00.000.log"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "email=u***@example.com user_code=***\n" {
			t.Fatalf("%s was not redacted: %q", name, got)
		}
	}
}
