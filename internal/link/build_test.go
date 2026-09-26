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
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/attest/attesttest"
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

func TestASignedBuildWithoutAKeyFailsBeforeResolvingImages(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	var work bytes.Buffer
	job := Job{Source: source, Out: filepath.Join(t.TempDir(), "myapp"), Platform: "linux/amd64"}
	if err := Build(context.Background(), report.New(&work), io.Discard, buildKinds(), job); !errors.Is(err, attest.ErrNoKey) {
		t.Fatalf("got %v, want ErrNoKey", err)
	}
	if strings.Contains(work.String(), "Resolving") {
		t.Fatal("resolved images before reading the key")
	}
}

func TestAnInsecureBuildWritesNoSidecar(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")

	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(out + attest.SignatureSuffix); !os.IsNotExist(err) {
		t.Errorf("got a sidecar at %s, want none", out+attest.SignatureSuffix)
	}
}

func TestAnInsecureBuildPrintsTheInsecureWarning(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	var stdout bytes.Buffer
	job := Job{Source: source, Out: filepath.Join(t.TempDir(), "myapp"), Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), &stdout, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(stdout.String(), bundle.InsecureWarning) {
		t.Errorf("got %q, want the insecure warning", stdout.String())
	}
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

func TestASignedBuildSignsTheLayoutItKeeps(t *testing.T) {
	layout, key := buildSignedLayout(t)
	if !attesttest.Verifies(t, key, filepath.Join(layout, "index.json")) {
		t.Error("the layout signature does not cover its index.json")
	}
}

func TestASignedBuildSignsTheExecutable(t *testing.T) {
	layout, key := buildSignedLayout(t)
	if !attesttest.Verifies(t, key, filepath.Join(filepath.Dir(layout), "myapp")) {
		t.Error("the signature does not cover the executable")
	}
}

// buildSignedLayout builds with --layout and a fresh key, and returns the layout and the key.
func buildSignedLayout(t *testing.T) (string, string) {
	t.Helper()
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	layout, key := filepath.Join(dir, "bundle"), attesttest.Key(t)
	job := Job{Source: source, Out: filepath.Join(dir, "myapp"), Layout: layout, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", Key: key}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	return layout, key
}

func TestAnUnsignedBuildLeavesTheLayoutUnsigned(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	layout := filepath.Join(dir, "bundle")
	job := Job{Source: source, Out: filepath.Join(dir, "myapp"), Layout: layout,
		Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout, "index.json"+attest.SignatureSuffix)); !os.IsNotExist(err) {
		t.Errorf("an unsigned build signed its layout, err=%v", err)
	}
}

func TestAnExecutableIsNeverPublishedWithoutItsSignature(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")
	blockSignature(t, out)
	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", Key: attesttest.Key(t)}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err == nil {
		t.Fatal("the build succeeded with no place to write its signature")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("the build published %s without its signature, err=%v", out, err)
	}
}

// blockSignature puts a directory where the signature of out goes, so writing it fails.
func blockSignature(t *testing.T, out string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(out+attest.SignatureSuffix, "taken"), 0o755); err != nil {
		t.Fatalf("block the signature path: %v", err)
	}
}

func TestASignedBuildWithoutLayoutSignsOnlyTheExecutable(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	out := filepath.Join(dir, "myapp")
	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", Key: attesttest.Key(t)}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if want := []string{"myapp", "myapp" + attest.SignatureSuffix}; !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestTheExecutableCarriesNoLayoutSignature(t *testing.T) {
	layout, _ := buildSignedLayout(t)
	payload := t.TempDir()
	if err := installer.Unpack(filepath.Join(filepath.Dir(layout), "myapp"), payload); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(payload, "index.json"+attest.SignatureSuffix)); !os.IsNotExist(err) {
		t.Errorf("the executable carries the layout signature, err=%v", err)
	}
}

func TestASecondBuildIntoTheSameLayoutSignsItAgain(t *testing.T) {
	layout, key := buildSignedLayout(t)
	source := t.TempDir()
	writeUnits(t, source)
	job := Job{Source: source, Out: filepath.Join(t.TempDir(), "myapp"), Layout: layout, Platform: "linux/amd64", Name: "acme", Version: "1.5.0", Key: key}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("second build: %v", err)
	}
	if !attesttest.Verifies(t, key, filepath.Join(layout, "index.json")) {
		t.Error("the layout keeps a signature of the first build")
	}
}

func TestAnUnsignedBuildDropsAnOldLayoutSignature(t *testing.T) {
	layout, _ := buildSignedLayout(t)
	source := t.TempDir()
	writeUnits(t, source)
	job := Job{Source: source, Out: filepath.Join(t.TempDir(), "myapp"), Layout: layout,
		Platform: "linux/amd64", Name: "acme", Version: "1.5.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("second build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout, "index.json"+attest.SignatureSuffix)); !os.IsNotExist(err) {
		t.Errorf("an unsigned build left a signature of another build, err=%v", err)
	}
}

func TestBuildFromABundlePacksThatExactBundle(t *testing.T) {
	layout := buildUnsignedLayout(t)
	before, err := bundle.Open(layout)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	out := filepath.Join(t.TempDir(), "myapp")
	job := Job{Source: layout, Out: out, InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build from the bundle: %v", err)
	}
	payload := t.TempDir()
	if err := installer.Unpack(out, payload); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	after, err := bundle.Open(payload)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if after.Root != before.Root {
		t.Errorf("got root %s, want the bundle's own %s", after.Root, before.Root)
	}
}

func TestBuildFromAnUnsignedBundleRefusesToSignIt(t *testing.T) {
	job := Job{Source: buildUnsignedLayout(t), Out: filepath.Join(t.TempDir(), "myapp"), Key: attesttest.Key(t)}
	err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job)
	if !errors.Is(err, ErrSigningMismatch) {
		t.Fatalf("got %v, want ErrSigningMismatch", err)
	}
}

func TestBuildFromASignedBundleRefusesToLeaveItUnsigned(t *testing.T) {
	layout, _ := buildSignedLayout(t)
	job := Job{Source: layout, Out: filepath.Join(t.TempDir(), "myapp"), InsecureUnsigned: true}
	err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job)
	if !errors.Is(err, ErrSigningMismatch) {
		t.Fatalf("got %v, want ErrSigningMismatch", err)
	}
}

func TestBuildFromABundleRefusesAFlagTheBundleAlreadyAnswers(t *testing.T) {
	layout := buildUnsignedLayout(t)
	for _, job := range []Job{
		{Name: "other"}, {Version: "9.9.9"}, {Layout: filepath.Join(t.TempDir(), "copy")}, {Platform: "linux/arm64"},
	} {
		job.Source, job.Out, job.InsecureUnsigned = layout, filepath.Join(t.TempDir(), "myapp"), true
		err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job)
		if !errors.Is(err, ErrBundleSourceFlags) {
			t.Errorf("%+v: got %v, want ErrBundleSourceFlags", job, err)
		}
	}
}

// buildUnsignedLayout builds with --layout --insecure-unsigned and returns the layout.
func buildUnsignedLayout(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	layout := filepath.Join(dir, "bundle")
	job := Job{Source: source, Out: filepath.Join(dir, "scratch"), Layout: layout,
		Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	return layout
}

func TestBuildWithEvidenceCarriesItInTheExecutable(t *testing.T) {
	out := filepath.Join(t.TempDir(), "myapp")
	job := Job{Source: buildUnsignedLayout(t), Out: out, InsecureUnsigned: true,
		Evidence: []string{"testdata/sbom.spdx.json"}}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	payload := t.TempDir()
	if err := installer.Unpack(out, payload); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	artifact, err := bundle.Open(payload)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(artifact.Evidence) != 1 || artifact.Evidence[0].Name != "sbom.spdx.json" {
		t.Errorf("got %+v, want the SBOM carried", artifact.Evidence)
	}
}

func TestBuildRefusesEvidenceForADescriptorItHasNotLinkedYet(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	job := Job{Source: source, Out: filepath.Join(t.TempDir(), "myapp"), Platform: "linux/amd64",
		Name: "acme", Version: "1.4.0", InsecureUnsigned: true, Evidence: []string{"testdata/sbom.spdx.json"}}
	var work bytes.Buffer
	err := Build(context.Background(), report.New(&work), io.Discard, buildKinds(), job)
	if !errors.Is(err, ErrEvidenceNeedsBundle) {
		t.Fatalf("got %v, want ErrEvidenceNeedsBundle", err)
	}
	if strings.Contains(work.String(), "Resolving") {
		t.Error("the refusal came after resolving images")
	}
}

func TestBuildWithEvidenceSignsTheNewIndex(t *testing.T) {
	layout, key := buildSignedLayout(t)
	job := Job{Source: layout, Out: filepath.Join(t.TempDir(), "myapp"), Evidence: []string{"testdata/sbom.spdx.json"}, Key: key}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("build: %v", err)
	}
	if !attesttest.Verifies(t, key, filepath.Join(layout, "index.json")) {
		t.Error("the layout signature covers the index before the evidence")
	}
}

func TestALinkWithNoPlatformResolvesForLinuxAmd64(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "bundle")
	job := Job{Source: source, Name: "acme", Version: "1.4.0"}
	if err := Link(context.Background(), report.New(io.Discard), report.New(io.Discard), buildKinds(), job, out); err != nil {
		t.Fatalf("link: %v", err)
	}
	artifact, err := bundle.Open(out)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if artifact.Config.Platform != "linux/amd64" {
		t.Errorf("got %q, want linux/amd64", artifact.Config.Platform)
	}
}

func TestBuildFromADamagedBundleSaysItIsDamaged(t *testing.T) {
	layout := buildUnsignedLayout(t)
	blobs, err := os.ReadDir(filepath.Join(layout, "blobs/sha256"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, blob := range blobs {
		if err := os.WriteFile(filepath.Join(layout, "blobs/sha256", blob.Name()), []byte("damaged"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	job := Job{Source: layout, Out: filepath.Join(t.TempDir(), "myapp"), InsecureUnsigned: true, Evidence: []string{"testdata/sbom.spdx.json"}}
	err = Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job)
	if !errors.Is(err, bundle.ErrDigestMismatch) {
		t.Fatalf("got %v, want bundle.ErrDigestMismatch", err)
	}
}

func TestBuildFromABundleChecksItsSigningModeBeforeReadingTheKey(t *testing.T) {
	job := Job{Source: buildUnsignedLayout(t), Out: filepath.Join(t.TempDir(), "myapp")}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); !errors.Is(err, ErrSigningMismatch) {
		t.Fatalf("got %v, want ErrSigningMismatch", err)
	}
}

func TestAnInterruptedEvidenceBuildCanRunAgain(t *testing.T) {
	layout, key := buildSignedLayout(t)
	out := filepath.Join(t.TempDir(), "myapp")
	job := Job{Source: layout, Out: out, Evidence: []string{"testdata/sbom.spdx.json"}, Key: key}
	blockSignature(t, out)
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err == nil {
		t.Fatal("the build succeeded with no place to write its signature")
	}
	if err := os.RemoveAll(out + attest.SignatureSuffix); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job); err != nil {
		t.Fatalf("the second run: %v", err)
	}
}
