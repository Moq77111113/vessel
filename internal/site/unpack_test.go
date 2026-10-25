package site

import (
	"archive/tar"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
)

func stub() []byte { return []byte("#!/bin/sh\necho a real binary would be here\n") }

func writePacked(t *testing.T, out string, entries map[string]string) {
	t.Helper()
	var payload bytes.Buffer
	archive := tar.NewWriter(&payload)
	for name, body := range entries {
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		if _, err := archive.Write([]byte(body)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	file := append(stub(), payload.Bytes()...)
	file = append(file, bundle.Trailer(int64(payload.Len()))...)
	if err := os.WriteFile(out, file, 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func packed(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "acme-1.4.0")
	writePacked(t, out, map[string]string{
		"vessel.json":       `{"name":"acme"}`,
		"images/index.json": `{"schemaVersion":2}`,
	})
	return out
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
	if err := Unpack(plain, t.TempDir()); !errors.Is(err, bundle.ErrNoPayload) {
		t.Errorf("got %v, want ErrNoPayload", err)
	}
}

func TestUnpackRefusesAPathThatLeavesTheRoot(t *testing.T) {
	out := filepath.Join(t.TempDir(), "evil")
	writePacked(t, out, map[string]string{"../escaped": "owned"})
	err := Unpack(out, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "..") {
		t.Errorf("got %v, want it to refuse the escaping path", err)
	}
}
