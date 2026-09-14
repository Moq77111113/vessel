package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/report"
)

func TestBuildWritesOneExecutableThatCarriesTheBundle(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")

	var work, stdout bytes.Buffer
	err := build(context.Background(), report.New(&work), &stdout,
		source, out, "", "linux/amd64", "acme", "1.4.0", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !installer.CarriesABundle(out) {
		t.Fatalf("%s carries no bundle", out)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat %s: %v", out, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("got mode %v, want an executable", info.Mode())
	}
}

func TestBuildWritesTheLayoutWhenAskedForIt(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	out := filepath.Join(dir, "myapp")
	layout := filepath.Join(dir, "bundle")

	var work, stdout bytes.Buffer
	err := build(context.Background(), report.New(&work), &stdout,
		source, out, layout, "linux/amd64", "acme", "1.4.0", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !bundle.IsBundle(layout) {
		t.Errorf("%s is not a bundle", layout)
	}
}

func TestBuildWithoutAKeyWarnsTheExecutableIsUnsigned(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")

	var work, stdout bytes.Buffer
	err := build(context.Background(), report.New(&work), &stdout,
		source, out, "", "linux/amd64", "acme", "1.4.0", "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(stdout.String(), "sha256sum myapp") {
		t.Errorf("stdout does not warn the file is unsigned: %s", stdout.String())
	}
}

func TestBuildWithAKeyNamesTheSignatureFile(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")
	key := writeKeyFile(t)

	var work, stdout bytes.Buffer
	err := build(context.Background(), report.New(&work), &stdout,
		source, out, "", "linux/amd64", "acme", "1.4.0", key)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(stdout.String(), out+".minisig, ship it alongside") {
		t.Errorf("stdout does not name the signature file: %s", stdout.String())
	}
}

// writeKeyFile writes an unencrypted minisign private key, for a test that signs.
func writeKeyFile(t *testing.T) string {
	t.Helper()
	_, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	text, err := private.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vessel.key")
	if err := os.WriteFile(path, text, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// writeUnits writes a quadlet unit under source, referencing an image seeded in a local registry.
func writeUnits(t *testing.T, source string) {
	t.Helper()
	host := registryHost(t)
	unit := "[Container]\nImage=" + host + "/acme/web:1.0\n"
	if err := os.WriteFile(filepath.Join(source, "web.container"), []byte(unit), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
