package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMkdirAllCreatesEveryMissingParentWithItsMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "var", "lib", "vessel")
	if err := MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Errorf("got %v, want a 0700 directory", info.Mode())
	}
}

func TestRenameMovesTheFile(t *testing.T) {
	dir := t.TempDir()
	from, to := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(from, []byte("a"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := Rename(from, to); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if _, err := os.Stat(from); !os.IsNotExist(err) {
		t.Errorf("the old name is still there: %v", err)
	}
	if body, err := os.ReadFile(to); err != nil || string(body) != "a" {
		t.Errorf("got %q, %v, want %q", body, err, "a")
	}
}
