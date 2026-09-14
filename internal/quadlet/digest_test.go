package quadlet

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

// realImageRef is the reference AddImages must land in podman's local storage.
const realImageRef = "vessel-quadlet-test.local/probe:1.0"

// The invariant the whole design rests on: the digest a unit pins is the digest of
// the image podman actually holds, not just the digest the bundle's own bytes carry.
func TestAddImagesLandsTheImagePodmanHoldsAtItsPinnedDigest(t *testing.T) {
	m := New(machine.Exec)
	skipUnlessPodmanReady(t, m)

	dir, digest := realImageLayout(t, realImageRef)
	lay, err := machine.OpenLayout(dir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	t.Cleanup(func() { removePodmanImage(t, realImageRef) })

	if _, err := m.AddImages(context.Background(), report.New(io.Discard), lay); err != nil {
		t.Fatalf("AddImages: %v", err)
	}

	if got := podmanImageDigest(t, realImageRef); got != digest {
		t.Errorf("got %s, want %s", got, digest)
	}
}

// skipUnlessPodmanReady skips naming exactly what podman lacks: missing, too old, or unable to run.
func skipUnlessPodmanReady(t *testing.T, m *Machine) {
	t.Helper()
	ctx := context.Background()
	version, err := m.podman.Version(ctx)
	if err != nil {
		t.Skipf("no podman: %v", err)
	}
	if err := CheckPodman(version); err != nil {
		t.Skipf("podman too old: %v", err)
	}
	if err := m.podman.Ready(ctx); err != nil {
		t.Skipf("podman cannot run: %v", err)
	}
}

// realImageLayout writes an OCI layout holding one real, podman-loadable image at ref,
// and returns its directory and the digest of the image manifest it carries.
func realImageLayout(t *testing.T, ref string) (string, string) {
	t.Helper()
	base := mutate.ConfigMediaType(mutate.MediaType(empty.Image, types.OCIManifestSchema1), types.OCIConfigJSON)
	body, err := random.Layer(128, types.OCILayer)
	if err != nil {
		t.Fatalf("random.Layer: %v", err)
	}
	image, err := mutate.AppendLayers(base, body)
	if err != nil {
		t.Fatalf("AppendLayers: %v", err)
	}
	digest, err := image.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}

	dir := t.TempDir()
	path, err := layout.Write(dir, empty.Index)
	if err != nil {
		t.Fatalf("layout.Write: %v", err)
	}
	annotations := map[string]string{machine.RefNameAnnotation: ref}
	if err := path.AppendImage(image, layout.WithAnnotations(annotations)); err != nil {
		t.Fatalf("AppendImage: %v", err)
	}
	return dir, digest.String()
}

// podmanImageDigest asks podman itself for the manifest digest it holds ref at.
func podmanImageDigest(t *testing.T, ref string) string {
	t.Helper()
	out, err := exec.Command("podman", "image", "inspect", "--format", "{{.Digest}}", ref).Output()
	if err != nil {
		t.Fatalf("podman image inspect %s: %v", ref, err)
	}
	return strings.TrimSpace(string(out))
}

// removePodmanImage drops ref from podman's local storage, cleaning up after the test.
func removePodmanImage(t *testing.T, ref string) {
	t.Helper()
	if err := exec.Command("podman", "image", "rm", "-f", ref).Run(); err != nil {
		t.Logf("podman image rm %s: %v", ref, err)
	}
}
