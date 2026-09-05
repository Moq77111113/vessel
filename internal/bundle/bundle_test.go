package bundle

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// imageLayout writes the OCI layout a fetch run would leave behind.
func imageLayout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, blobsDir), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	write(layoutName, `{"imageLayoutVersion":"1.0.0"}`)
	write(indexName, `{"schemaVersion":2,"mediaType":"`+indexType+`","manifests":[`+
		`{"mediaType":"`+manifestType+`","digest":"sha256:aaa","size":1,`+
		`"annotations":{"org.opencontainers.image.ref.name":"reg.io/app@sha256:aaa"}}]}`)
	write(filepath.Join(blobsDir, "aaa"), "a manifest")
	return dir
}

func writing(t *testing.T) Writing {
	t.Helper()
	return Writing{
		Layout: imageLayout(t),
		Config: Config{
			Name: "acme", Version: "1.4.0", Reader: "quadlet", Platform: "linux/amd64",
			Images: []Image{{Ref: "reg.io/app:1.0", Digest: "sha256:aaa"}},
		},
		Files: []descriptor.File{
			{Path: "etc/containers/systemd/app.container", Data: []byte("[Container]\nImage=reg.io/app@sha256:aaa\n")},
			{Path: "etc/containers/systemd/app.network", Data: []byte("[Network]\nNetworkName=app\n")},
		},
	}
}

func built(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	if err := Write(dir, writing(t)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dir
}

func TestABundleIsAnOciLayout(t *testing.T) {
	dir := built(t)
	for _, name := range []string{layoutName, indexName} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func topIndex(t *testing.T, dir string) index {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, indexName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var carried index
	if err := json.Unmarshal(body, &carried); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return carried
}

func TestTheTopIndexPointsAtOneThingOnly(t *testing.T) {
	carried := topIndex(t, built(t))
	if got, want := len(carried.Manifests), 1; got != want {
		t.Fatalf("manifests: got %d, want %d, the bundle root", got, want)
	}
	if got, want := carried.Manifests[0].ArtifactType, ArtifactType; got != want {
		t.Errorf("artifactType: got %q, want %q", got, want)
	}
	if got, want := carried.Manifests[0].MediaType, indexType; got != want {
		t.Errorf("the root must be an index so its children travel with it: got %q, want %q", got, want)
	}
}

func TestTheBundleRootReferencesTheImagesAndTheFiles(t *testing.T) {
	dir := built(t)
	body, err := readBlob(dir, topIndex(t, dir).Manifests[0].Digest)
	if err != nil {
		t.Fatalf("readBlob: %v", err)
	}
	var root index
	if err := json.Unmarshal(body, &root); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got, want := len(root.Manifests), 2; got != want {
		t.Fatalf("manifests: got %d, want %d (one image, one files)", got, want)
	}
	kinds := map[string]bool{}
	for _, entry := range root.Manifests {
		kinds[entry.ArtifactType] = true
	}
	if !kinds[FilesType] {
		t.Errorf("the bundle root does not reference the files manifest: %v", kinds)
	}
}

func TestEveryBlobSitsUnderItsOwnDigest(t *testing.T) {
	dir := built(t)
	entries, err := os.ReadDir(filepath.Join(dir, blobsDir))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() == "aaa" {
			continue
		}
		if _, err := readBlob(dir, "sha256:"+entry.Name()); err != nil {
			t.Errorf("%s: %v", entry.Name(), err)
		}
	}
}

func TestOpenReturnsTheFilesWriteCarried(t *testing.T) {
	opened, err := Open(built(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := len(opened.Files), 2; got != want {
		t.Fatalf("files: got %d, want %d", got, want)
	}
	if got, want := opened.Files[0].Path, "etc/containers/systemd/app.container"; got != want {
		t.Errorf("path: got %q, want %q", got, want)
	}
}

func TestOpenReturnsTheConfigWriteCarried(t *testing.T) {
	opened, err := Open(built(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := opened.Config.Name, "acme"; got != want {
		t.Errorf("name: got %q, want %q", got, want)
	}
	if got, want := opened.Config.Images[0].Digest, "sha256:aaa"; got != want {
		t.Errorf("digest: got %q, want %q", got, want)
	}
}

func TestTheRootIsTheBundleManifestDigest(t *testing.T) {
	dir := built(t)
	opened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	root, err := RootBytes(dir)
	if err != nil {
		t.Fatalf("RootBytes: %v", err)
	}
	if got, want := string(root), opened.Root; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !strings.HasPrefix(opened.Root, "sha256:") {
		t.Errorf("the root is not a digest: %q", opened.Root)
	}
}

func TestOpenRefusesATamperedBlob(t *testing.T) {
	dir := built(t)
	opened, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	name := strings.TrimPrefix(opened.Root, "sha256:")
	if err := os.WriteFile(filepath.Join(dir, blobsDir, name), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := Open(dir); !errors.Is(err, ErrDigestMismatch) {
		t.Errorf("got %v, want ErrDigestMismatch", err)
	}
}

func TestOpenRefusesADirectoryThatIsNotABundle(t *testing.T) {
	if _, err := Open(t.TempDir()); !errors.Is(err, ErrNotABundle) {
		t.Errorf("got %v, want ErrNotABundle", err)
	}
}

func TestIsBundleIsFalseOnAPlainImageLayout(t *testing.T) {
	if IsBundle(imageLayout(t)) {
		t.Error("IsBundle: want false on a layout with no bundle manifest")
	}
}

func TestTheBundleRootCarriesATagSoAToolCanAddressIt(t *testing.T) {
	for _, entry := range topIndex(t, built(t)).Manifests {
		if entry.ArtifactType != ArtifactType {
			continue
		}
		if got, want := entry.Annotations[refNameAnnotation], "acme:1.4.0"; got != want {
			t.Errorf("tag: got %q, want %q", got, want)
		}
		return
	}
	t.Fatal("no bundle manifest in the index")
}
