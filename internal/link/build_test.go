package link

import (
	"net/http/httptest"
	"net/url"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
)

func TestBuildWritesAFileThatCarriesTheBundle(t *testing.T) {
	out := buildTestApp(t, "")
	if !installer.CarriesABundle(out) {
		t.Errorf("%s carries no bundle", out)
	}
}

func TestBuildWritesAFileTheMachineCanRun(t *testing.T) {
	out := buildTestApp(t, "")
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat %s: %v", out, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("got mode %v, want an executable", info.Mode())
	}
}

// buildTestApp builds one executable from a one-unit source and returns its path.
func buildTestApp(t *testing.T, key string) string {
	t.Helper()
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")
	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", Key: key}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	return out
}

func TestBuildWritesTheLayoutWhenAskedForIt(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	out := filepath.Join(dir, "myapp")
	layout := filepath.Join(dir, "bundle")

	var stdout bytes.Buffer
	job := Job{Source: source, Out: out, Layout: layout, Platform: "linux/amd64", Name: "acme", Version: "1.4.0"}
	if err := Build(context.Background(), report.New(io.Discard), &stdout, buildKinds(), job); err != nil {
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

	var stdout bytes.Buffer
	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0"}
	if err := Build(context.Background(), report.New(io.Discard), &stdout, buildKinds(), job); err != nil {
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

	var stdout bytes.Buffer
	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", Key: key}
	if err := Build(context.Background(), report.New(io.Discard), &stdout, buildKinds(), job); err != nil {
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

// buildKinds is the machine list a build links through: quadlet reads the units, nothing runs.
func buildKinds() []machine.Machine {
	return []machine.Machine{quadlet.New(machine.Exec)}
}

// registryHost starts a local registry seeded with one image and returns its host:port.
func registryHost(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(ggcrregistry.New())
	t.Cleanup(server.Close)
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	image, err := random.Image(128, 1)
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}
	tag, err := name.NewTag(address.Host + "/acme/web:1.0")
	if err != nil {
		t.Fatalf("NewTag: %v", err)
	}
	if err := remote.Write(tag, image); err != nil {
		t.Fatalf("remote.Write: %v", err)
	}
	return address.Host
}
