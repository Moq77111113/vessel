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
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
)

func TestBuildWritesAFileThatCarriesTheBundle(t *testing.T) {
	out := buildTestApp(t)
	if !installer.CarriesABundle(out) {
		t.Errorf("%s carries no bundle", out)
	}
}

func TestBuildWritesAFileTheMachineCanRun(t *testing.T) {
	out := buildTestApp(t)
	info, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat %s: %v", out, err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("got mode %v, want an executable", info.Mode())
	}
}

func buildTestApp(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")
	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
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
	job := Job{Source: source, Out: out, Layout: layout, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), &stdout, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	if !bundle.IsBundle(layout) {
		t.Errorf("%s is not a bundle", layout)
	}
}

func TestBuildWithoutCIIdentityFailsBeforeResolvingImages(t *testing.T) {
	clearCIIdentity(t)
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")

	var work bytes.Buffer
	job := Job{Source: source, Out: out, Platform: "linux/amd64"}
	err := Build(context.Background(), report.New(&work), io.Discard, buildKinds(), job)
	if !errors.Is(err, attest.ErrNoOIDCToken) {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(work.String(), "Resolving") {
		t.Fatal("resolved images before signing preflight")
	}
}

func TestInsecureUnsignedBuildLeavesNoSidecar(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")

	var stdout bytes.Buffer
	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), &stdout, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(out + attest.SigstoreSuffix); !os.IsNotExist(err) {
		t.Errorf("got a sidecar at %s, want none", out+attest.SigstoreSuffix)
	}
	if !strings.Contains(stdout.String(), "INSECURE") {
		t.Errorf("stdout does not warn the build is insecure: %s", stdout.String())
	}
}

func clearCIIdentity(t *testing.T) {
	t.Helper()
	t.Setenv("VESSEL_SIGSTORE_ID_TOKEN", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "")
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "")
}

func writeUnits(t *testing.T, source string) {
	t.Helper()
	host := registryHost(t)
	unit := "[Container]\nImage=" + host + "/acme/web:1.0\n"
	if err := os.WriteFile(filepath.Join(source, "web.container"), []byte(unit), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func buildKinds() []machine.Machine {
	return []machine.Machine{quadlet.New(machine.Exec)}
}

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

func TestAnInsecureBuildSaysSoInTheBundle(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	layout := filepath.Join(dir, "bundle")
	job := Job{Source: source, Out: filepath.Join(dir, "myapp"), Layout: layout,
		Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	artifact, err := bundle.Open(layout)
	if err != nil {
		t.Fatalf("open the bundle: %v", err)
	}
	if !artifact.Config.Insecure {
		t.Error("the bundle of an unsigned build does not say it is insecure")
	}
}

func TestASignedLinkWritesNoInsecureKey(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "bundle")
	job := Job{Source: source, Platform: "linux/amd64", Name: "acme", Version: "1.4.0"}
	if err := Link(context.Background(), report.New(io.Discard), report.New(io.Discard), buildKinds(), job, out); err != nil {
		t.Fatalf("link: %v", err)
	}
	blobs, err := os.ReadDir(filepath.Join(out, "blobs/sha256"))
	if err != nil {
		t.Fatalf("read the blobs: %v", err)
	}
	for _, entry := range blobs {
		body, err := os.ReadFile(filepath.Join(out, "blobs/sha256", entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		if bytes.Contains(body, []byte(`"insecure"`)) {
			t.Errorf("blob %s carries an insecure key for a signed link", entry.Name())
		}
	}
}
