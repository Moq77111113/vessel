package attest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeSigner struct {
	bundle []byte
	err    error
}

func (s fakeSigner) Sign(context.Context, []byte) ([]byte, error) {
	return s.bundle, s.err
}

func TestSignFileWritesTheSidecar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "myapp")
	if err := os.WriteFile(path, []byte("installer bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sidecar, err := SignFile(context.Background(), path, fakeSigner{bundle: []byte("bundle bytes")})
	if err != nil {
		t.Fatalf("SignFile: %v", err)
	}
	if want := path + SigstoreSuffix; sidecar != want {
		t.Errorf("sidecar path = %q, want %q", sidecar, want)
	}

	got, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "bundle bytes" {
		t.Errorf("sidecar content = %q, want %q", got, "bundle bytes")
	}
}

func TestSignFileLeavesNoSidecarOnSignerError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "myapp")
	if err := os.WriteFile(path, []byte("installer bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	signErr := errors.New("boom")

	_, err := SignFile(context.Background(), path, fakeSigner{err: signErr})
	if !errors.Is(err, signErr) {
		t.Fatalf("got %v, want %v", err, signErr)
	}

	if _, statErr := os.Stat(path + SigstoreSuffix); !os.IsNotExist(statErr) {
		t.Errorf("sidecar exists after signer error: %v", statErr)
	}
}

func TestSignFileRejectsAnUnreadablePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist")

	_, err := SignFile(context.Background(), path, fakeSigner{bundle: []byte("bundle bytes")})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(path + SigstoreSuffix); !os.IsNotExist(statErr) {
		t.Errorf("sidecar exists after read error: %v", statErr)
	}
}
