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
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
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
	if err := Build(context.Background(), report.New(io.Discard), &stdout, buildKinds(), job, fakeSigning); err != nil {
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
	err := Build(context.Background(), report.New(&work), io.Discard, buildKinds(), job, Sigstore)
	if !errors.Is(err, attest.ErrNoOIDCToken) {
		t.Fatalf("Build: %v", err)
	}
	if strings.Contains(work.String(), "Resolving") {
		t.Fatal("resolved images before signing preflight")
	}
}

func TestAnInsecureBuildWritesNoSidecar(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	out := filepath.Join(t.TempDir(), "myapp")

	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(out + attest.SigstoreSuffix); !os.IsNotExist(err) {
		t.Errorf("got a sidecar at %s, want none", out+attest.SigstoreSuffix)
	}
}

func TestAnInsecureBuildPrintsTheInsecureWarning(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	var stdout bytes.Buffer
	job := Job{Source: source, Out: filepath.Join(t.TempDir(), "myapp"), Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), &stdout, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(stdout.String(), bundle.InsecureWarning) {
		t.Errorf("got %q, want the insecure warning", stdout.String())
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
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
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
	layout := buildSignedLayout(t)
	index, err := os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	sidecar, err := os.ReadFile(filepath.Join(layout, "index.json"+attest.SigstoreSuffix))
	if err != nil {
		t.Fatalf("the layout carries no signature: %v", err)
	}
	if string(sidecar) != "signature over "+string(index) {
		t.Error("the layout signature does not cover its index.json")
	}
}

// buildSignedLayout builds with --layout and the fake signer, and returns the layout.
func buildSignedLayout(t *testing.T) string {
	t.Helper()
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	layout := filepath.Join(dir, "bundle")
	job := Job{Source: source, Out: filepath.Join(dir, "myapp"), Layout: layout, Platform: "linux/amd64", Name: "acme", Version: "1.4.0"}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("build: %v", err)
	}
	return layout
}

// fakeSigner signs by prefixing the bytes, so a test can tell what a signature covers.
type fakeSigner struct{}

func (fakeSigner) Sign(_ context.Context, data []byte) ([]byte, error) {
	return append([]byte("signature over "), data...), nil
}

func fakeSigning(context.Context) (attest.ArtifactSigner, error) { return fakeSigner{}, nil }

func TestAnUnsignedBuildLeavesTheLayoutUnsigned(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	layout := filepath.Join(dir, "bundle")
	job := Job{Source: source, Out: filepath.Join(dir, "myapp"), Layout: layout,
		Platform: "linux/amd64", Name: "acme", Version: "1.4.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout, "index.json"+attest.SigstoreSuffix)); !os.IsNotExist(err) {
		t.Errorf("an unsigned build signed its layout, err=%v", err)
	}
}

func TestAFailedLayoutSignatureLeavesNoExecutable(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	out := filepath.Join(dir, "myapp")
	job := Job{Source: source, Out: out, Layout: filepath.Join(dir, "bundle"), Platform: "linux/amd64", Name: "acme", Version: "1.4.0"}
	err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, failingSigning)
	if !errors.Is(err, errSignerDown) {
		t.Fatalf("got %v, want errSignerDown", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Errorf("the build published %s with an unsigned layout, err=%v", out, err)
	}
}

var errSignerDown = errors.New("signer down")

type failingSigner struct{}

func (failingSigner) Sign(context.Context, []byte) ([]byte, error) { return nil, errSignerDown }

func failingSigning(context.Context) (attest.ArtifactSigner, error) { return failingSigner{}, nil }

func TestASignedBuildWithoutLayoutSignsOnlyTheExecutable(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	out := filepath.Join(dir, "myapp")
	job := Job{Source: source, Out: out, Platform: "linux/amd64", Name: "acme", Version: "1.4.0"}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
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
	if want := []string{"myapp", "myapp" + attest.SigstoreSuffix}; !slices.Equal(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestTheExecutableCarriesNoLayoutSignature(t *testing.T) {
	layout := buildSignedLayout(t)
	payload := t.TempDir()
	if err := installer.Unpack(filepath.Join(filepath.Dir(layout), "myapp"), payload); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(payload, "index.json"+attest.SigstoreSuffix)); !os.IsNotExist(err) {
		t.Errorf("the executable carries the layout signature, err=%v", err)
	}
}

func TestASecondBuildIntoTheSameLayoutSignsItAgain(t *testing.T) {
	layout := buildSignedLayout(t)
	source := t.TempDir()
	writeUnits(t, source)
	job := Job{Source: source, Out: filepath.Join(t.TempDir(), "myapp"), Layout: layout, Platform: "linux/amd64", Name: "acme", Version: "1.5.0"}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("second build: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	sidecar, err := os.ReadFile(filepath.Join(layout, "index.json"+attest.SigstoreSuffix))
	if err != nil {
		t.Fatalf("read the signature: %v", err)
	}
	if string(sidecar) != "signature over "+string(index) {
		t.Error("the layout keeps a signature of the first build")
	}
	payload := t.TempDir()
	if err := installer.Unpack(job.Out, payload); err != nil {
		t.Fatalf("unpack: %v", err)
	}
	if _, err := os.Stat(filepath.Join(payload, "index.json"+attest.SigstoreSuffix)); !os.IsNotExist(err) {
		t.Errorf("the second executable carries the first layout signature, err=%v", err)
	}
}

func TestAnUnsignedBuildDropsAnOldLayoutSignature(t *testing.T) {
	layout := buildSignedLayout(t)
	source := t.TempDir()
	writeUnits(t, source)
	job := Job{Source: source, Out: filepath.Join(t.TempDir(), "myapp"), Layout: layout,
		Platform: "linux/amd64", Name: "acme", Version: "1.5.0", InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("second build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(layout, "index.json"+attest.SigstoreSuffix)); !os.IsNotExist(err) {
		t.Errorf("an unsigned build left a signature of another build, err=%v", err)
	}
}

func TestAFailedSignatureLeavesNoSignedLayout(t *testing.T) {
	source := t.TempDir()
	writeUnits(t, source)
	dir := t.TempDir()
	out := filepath.Join(dir, "myapp")
	layout := filepath.Join(dir, "bundle")
	job := Job{Source: source, Out: out, Layout: layout, Platform: "linux/amd64", Name: "acme", Version: "1.4.0"}
	signer := &secondCallFails{}
	signing := func(context.Context) (attest.ArtifactSigner, error) { return signer, nil }
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, signing); !errors.Is(err, errSignerDown) {
		t.Fatalf("got %v, want errSignerDown", err)
	}
	for _, path := range []string{out, filepath.Join(layout, "index.json"+attest.SigstoreSuffix)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("a failed build left %s, err=%v", path, err)
		}
	}
}

// secondCallFails signs once, then fails, as a token expiring between two signatures would.
type secondCallFails struct {
	calls int
}

func (s *secondCallFails) Sign(_ context.Context, data []byte) ([]byte, error) {
	s.calls++
	if s.calls > 1 {
		return nil, errSignerDown
	}
	return append([]byte("signature over "), data...), nil
}

func TestBuildFromABundlePacksThatExactBundle(t *testing.T) {
	layout := buildUnsignedLayout(t)
	before, err := bundle.Open(layout)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	out := filepath.Join(t.TempDir(), "myapp")
	job := Job{Source: layout, Out: out, InsecureUnsigned: true}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
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
	job := Job{Source: buildUnsignedLayout(t), Out: filepath.Join(t.TempDir(), "myapp")}
	err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning)
	if !errors.Is(err, ErrSigningMismatch) {
		t.Fatalf("got %v, want ErrSigningMismatch", err)
	}
}

func TestBuildFromASignedBundleRefusesToLeaveItUnsigned(t *testing.T) {
	job := Job{Source: buildSignedLayout(t), Out: filepath.Join(t.TempDir(), "myapp"), InsecureUnsigned: true}
	err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning)
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
		err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning)
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
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("build: %v", err)
	}
	return layout
}

func TestBuildWithEvidenceCarriesItInTheExecutable(t *testing.T) {
	out := filepath.Join(t.TempDir(), "myapp")
	job := Job{Source: buildUnsignedLayout(t), Out: out, InsecureUnsigned: true,
		Evidence: []string{"testdata/sbom.spdx.json"}}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
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
	err := Build(context.Background(), report.New(&work), io.Discard, buildKinds(), job, fakeSigning)
	if !errors.Is(err, ErrEvidenceNeedsBundle) {
		t.Fatalf("got %v, want ErrEvidenceNeedsBundle", err)
	}
	if strings.Contains(work.String(), "Resolving") {
		t.Error("the refusal came after resolving images")
	}
}

func TestBuildWithEvidenceSignsTheNewIndex(t *testing.T) {
	layout := buildSignedLayout(t)
	job := Job{Source: layout, Out: filepath.Join(t.TempDir(), "myapp"),
		Evidence: []string{"testdata/sbom.spdx.json"}}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("build: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(layout, "index.json"))
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	sidecar, err := os.ReadFile(filepath.Join(layout, "index.json"+attest.SigstoreSuffix))
	if err != nil {
		t.Fatalf("read the signature: %v", err)
	}
	if string(sidecar) != "signature over "+string(index) {
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
	err = Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning)
	if !errors.Is(err, bundle.ErrDigestMismatch) {
		t.Fatalf("got %v, want bundle.ErrDigestMismatch", err)
	}
}

func TestBuildFromABundleChecksItsSigningModeBeforeAskingForAnIdentity(t *testing.T) {
	job := Job{Source: buildUnsignedLayout(t), Out: filepath.Join(t.TempDir(), "myapp")}
	err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, noIdentity)
	if !errors.Is(err, ErrSigningMismatch) {
		t.Fatalf("got %v, want ErrSigningMismatch", err)
	}
}

func TestAnInterruptedEvidenceBuildCanRunAgain(t *testing.T) {
	layout := buildSignedLayout(t)
	job := Job{Source: layout, Out: filepath.Join(t.TempDir(), "myapp"), Evidence: []string{"testdata/sbom.spdx.json"}}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, failingSigning); err == nil {
		t.Fatal("the build succeeded with a signer that is down")
	}
	if err := Build(context.Background(), report.New(io.Discard), io.Discard, buildKinds(), job, fakeSigning); err != nil {
		t.Fatalf("the second run: %v", err)
	}
}

func noIdentity(context.Context) (attest.ArtifactSigner, error) { return nil, attest.ErrNoOIDCToken }
