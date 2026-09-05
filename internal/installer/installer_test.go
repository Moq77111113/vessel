package installer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stub() []byte { return []byte("#!/bin/sh\necho a real binary would be here\n") }

func bundleDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for path, body := range map[string]string{
		"vessel.json":       `{"name":"acme"}`,
		"images/index.json": `{"schemaVersion":2}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return dir
}

func packed(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "acme-1.4.0")
	if err := Pack(bytes.NewReader(stub()), bundleDir(t), out); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return out
}

func TestPackKeepsTheStubRunnable(t *testing.T) {
	data, err := os.ReadFile(packed(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.HasPrefix(data, stub()) {
		t.Error("the stub is no longer at the front of the file")
	}
}

func TestPackMakesTheResultExecutable(t *testing.T) {
	info, err := os.Stat(packed(t))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("mode %v, want it executable", info.Mode())
	}
}

func TestUnpackReturnsTheFilesPackCarried(t *testing.T) {
	root := t.TempDir()
	if err := Unpack(packed(t), root); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "vessel.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got, want := string(body), `{"name":"acme"}`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnpackKeepsTheDirectoriesPackCarried(t *testing.T) {
	root := t.TempDir()
	if err := Unpack(packed(t), root); err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "images", "index.json")); err != nil {
		t.Errorf("the nested file is missing: %v", err)
	}
}

func TestCarriesABundleIsTrueOnAPackedFile(t *testing.T) {
	if !CarriesABundle(packed(t)) {
		t.Error("CarriesABundle: want true on a packed file")
	}
}

func TestCarriesABundleIsFalseOnAPlainBinary(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "vessel")
	if err := os.WriteFile(plain, stub(), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if CarriesABundle(plain) {
		t.Error("CarriesABundle: want false on a plain binary")
	}
}

func TestUnpackRefusesAFileCarryingNothing(t *testing.T) {
	plain := filepath.Join(t.TempDir(), "vessel")
	if err := os.WriteFile(plain, stub(), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := Unpack(plain, t.TempDir()); !errors.Is(err, ErrNoPayload) {
		t.Errorf("got %v, want ErrNoPayload", err)
	}
}

func TestUnpackRefusesAPathThatLeavesTheRoot(t *testing.T) {
	out := filepath.Join(t.TempDir(), "evil")
	if err := packEscape(out); err != nil {
		t.Fatalf("packEscape: %v", err)
	}
	err := Unpack(out, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "..") {
		t.Errorf("got %v, want it to refuse the escaping path", err)
	}
}
