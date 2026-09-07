package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// anImage builds a pseudo-random image and the reference pinning it to its own digest.
func anImage(t *testing.T, repository string) (v1.Image, descriptor.Ref) {
	t.Helper()
	image, err := random.Image(256, 1)
	if err != nil {
		t.Fatalf("random.Image: %v", err)
	}
	digest, err := image.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	ref, err := descriptor.ParseRef(repository + "@" + digest.String())
	if err != nil {
		t.Fatalf("ParseRef: %v", err)
	}
	return image, ref
}

// The invariant CLAUDE.md names: the digest a unit pins is the digest of the manifest blob the
// bundle carries, byte for byte.
func TestWriteLayoutWritesTheBlobTheImageDigestNames(t *testing.T) {
	image, ref := anImage(t, "registry.test/acme/web")
	dir := t.TempDir()
	if err := WriteLayout(dir, ref, image); err != nil {
		t.Fatalf("WriteLayout: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "blobs", "sha256", strings.TrimPrefix(ref.Digest, "sha256:")))
	if err != nil {
		t.Fatalf("no blob named after the digest the reference pins: %v", err)
	}
	sum := sha256.Sum256(body)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != ref.Digest {
		t.Errorf("blob hashes to %s, the reference pins %s", got, ref.Digest)
	}
}

func TestWriteLayoutCreatesTheLayoutWhenTheDirectoryHoldsNone(t *testing.T) {
	image, ref := anImage(t, "registry.test/acme/web")
	dir := t.TempDir()
	if err := WriteLayout(dir, ref, image); err != nil {
		t.Fatalf("WriteLayout: %v", err)
	}
	for _, name := range []string{"index.json", "oci-layout"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("no %s in the layout: %v", name, err)
		}
	}
}

func TestWriteLayoutAnnotatesTheManifestWithTheWholeReference(t *testing.T) {
	image, ref := anImage(t, "registry.test/acme/web")
	dir := t.TempDir()
	if err := WriteLayout(dir, ref, image); err != nil {
		t.Fatalf("WriteLayout: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(index), ref.String()) {
		t.Errorf("index.json does not carry %q: %s", ref, index)
	}
}

func TestWriteLayoutLeavesTwoImagesInOneLayout(t *testing.T) {
	dir := t.TempDir()
	for _, repository := range []string{"registry.test/acme/web", "registry.test/library/postgres"} {
		image, ref := anImage(t, repository)
		if err := WriteLayout(dir, ref, image); err != nil {
			t.Fatalf("WriteLayout %s: %v", repository, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var index struct {
		Manifests []struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(body, &index); err != nil {
		t.Fatalf("decode index.json: %v", err)
	}
	if got := len(index.Manifests); got != 2 {
		t.Fatalf("manifests: got %d, want 2", got)
	}
	for _, manifest := range index.Manifests {
		if manifest.Annotations[refNameAnnotation] == "" {
			t.Errorf("a manifest carries no %s annotation", refNameAnnotation)
		}
	}
}
