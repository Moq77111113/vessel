package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/target"
)

// The invariant the whole design rests on: the digest a unit pins is the digest of
// the manifest blob the bundle carries, byte for byte. A docker-archive round trip
// breaks it, which is why the bundle holds an OCI layout.
func TestEveryPinnedDigestNamesAManifestTheBundleCarries(t *testing.T) {
	opened, err := bundle.Open(linkStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(opened.Config.Images) == 0 {
		t.Fatal("the lock is empty")
	}
	for _, image := range opened.Config.Images {
		name := strings.TrimPrefix(image.Digest, "sha256:")
		body, err := os.ReadFile(filepath.Join(opened.LayoutDir, "blobs", "sha256", name))
		if err != nil {
			t.Fatalf("%s: no manifest blob in the bundle: %v", image.Ref, err)
		}
		sum := sha256.Sum256(body)
		if got := "sha256:" + hex.EncodeToString(sum[:]); got != image.Digest {
			t.Errorf("%s: blob hashes to %s, the lock pins %s", image.Ref, got, image.Digest)
		}
	}
}

// The two fixture images share one layer. A layout stores it once; that is the whole
// reason a bundle carries a layout rather than one archive per image.
func TestTwoImagesSharingALayerStoreItOnce(t *testing.T) {
	opened, err := bundle.Open(linkStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(opened.LayoutDir, "blobs", "sha256"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	// Per image: a manifest, a config, its own layer. Plus the shared layer, once.
	// Plus vessel's four: the bundle index, the files manifest, its config, the file archive.
	want := 3*len(images) + 1 + 4
	if len(entries) != want {
		t.Errorf("blobs: got %d, want %d; the shared layer is stored more than once", len(entries), want)
	}
}

// A unit written Image = ref, with spaces around the equals, still resolves: the
// spaced key is legal systemd syntax and must not slip an image through unpinned.
func TestLinkOverAUnitWrittenWithSpacesProducesADigest(t *testing.T) {
	host := serveRegistry(t)
	source := t.TempDir()
	unit := fmt.Sprintf("[Container]\nImage = %s/acme/web:1.0\n", host)
	if err := os.WriteFile(filepath.Join(source, "web.container"), []byte(unit), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	out := filepath.Join(t.TempDir(), "bundle")
	if err := runVessel("link", "-o", out, "--name", "acme", "--version", "1.0", source); err != nil {
		t.Fatalf("link: %v", err)
	}
	opened, err := bundle.Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, file := range opened.Files {
		if !strings.HasSuffix(file.Path, "web.container") {
			continue
		}
		body := string(file.Data)
		if !strings.Contains(body, "@sha256:") {
			t.Errorf("still carries a tag, not a digest: %s", body)
		}
	}
}

func TestEveryImageCutsIntoItsOwnArchive(t *testing.T) {
	opened, err := bundle.Open(linkStack(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	layout, err := target.OpenLayout(opened.LayoutDir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	if got, want := layout.Count(), len(images); got != want {
		t.Fatalf("images: got %d, want %d", got, want)
	}
	for i := range layout.Count() {
		var archive strings.Builder
		if err := layout.Archive(i, &archive); err != nil {
			t.Fatalf("Archive(%d): %v", i, err)
		}
		if archive.Len() == 0 {
			t.Errorf("Archive(%d) wrote nothing", i)
		}
	}
}
