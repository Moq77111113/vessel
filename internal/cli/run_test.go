package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAMachineWithoutTheScratchDirectoryKeepsItsTemporaryDirectory(t *testing.T) {
	t.Setenv("TMPDIR", "")
	useScratch(filepath.Join(t.TempDir(), "missing"))
	if got := os.Getenv("TMPDIR"); got != "" {
		t.Errorf("got TMPDIR %q, want it left unset", got)
	}
}

func TestAMachineWithTheScratchDirectoryUnpacksThere(t *testing.T) {
	scratch := t.TempDir()
	t.Setenv("TMPDIR", "")
	useScratch(scratch)
	if got := os.Getenv("TMPDIR"); got != scratch {
		t.Errorf("got TMPDIR %q, want %q", got, scratch)
	}
}
