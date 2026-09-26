package bundle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestABundleCarriesTheEvidenceAttachedToIt(t *testing.T) {
	dir := emptyBundle(t)
	if err := Attach(dir, []Evidence{sbom(t)}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	artifact, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(artifact.Evidence) != 1 || artifact.Evidence[0].Name != "sbom.spdx.json" ||
		string(artifact.Evidence[0].Data) != string(sbom(t).Data) {
		t.Errorf("got %+v, want the one SBOM", artifact.Evidence)
	}
}

func TestAttachingEvidenceLeavesTheRootAlone(t *testing.T) {
	dir := emptyBundle(t)
	before, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := Attach(dir, []Evidence{sbom(t)}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	after, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if before.Root != after.Root {
		t.Errorf("got root %s, want %s unchanged", after.Root, before.Root)
	}
}

func TestAttachRefusesANameThatIsAPath(t *testing.T) {
	for _, name := range []string{"", ".", "..", "a/b", "../x"} {
		err := Attach(emptyBundle(t), []Evidence{{Name: name, Data: []byte("{}")}})
		if !errors.Is(err, ErrEvidenceName) {
			t.Errorf("%q: got %v, want ErrEvidenceName", name, err)
		}
	}
}

func TestAttachRefusesANameTheBundleCarriesWithOtherContent(t *testing.T) {
	dir := emptyBundle(t)
	if err := Attach(dir, []Evidence{sbom(t)}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := Attach(dir, []Evidence{{Name: "sbom.spdx.json", Data: []byte("other")}}); !errors.Is(err, ErrEvidenceTwice) {
		t.Fatalf("got %v, want ErrEvidenceTwice", err)
	}
}

func TestOpenRefusesEvidenceNamedToLeaveADirectory(t *testing.T) {
	dir := emptyBundle(t)
	root, err := bundleEntry(dir)
	if err != nil {
		t.Fatalf("bundleEntry: %v", err)
	}
	if err := attach(dir, root, []Evidence{{Name: "../../etc/cron.d/x", Data: []byte("x")}}); err != nil {
		t.Fatalf("attach without checks: %v", err)
	}
	if _, err := Open(dir); !errors.Is(err, ErrEvidenceName) {
		t.Fatalf("got %v, want ErrEvidenceName", err)
	}
}

// emptyBundle writes a bundle with no image and no file over testdata/empty-layout.
func emptyBundle(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := Contents{
		Layout: "testdata/empty-layout",
		Config: Config{Name: "acme", Version: "1.4.0", Machine: "quadlet", Platform: "linux/amd64"},
	}
	if err := Write(dir, contents); err != nil {
		t.Fatalf("write the bundle: %v", err)
	}
	return dir
}

// sbom is testdata/sbom.spdx.json as one piece of evidence.
func sbom(t *testing.T) Evidence {
	t.Helper()
	body, err := os.ReadFile("testdata/sbom.spdx.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	return Evidence{Name: "sbom.spdx.json", Data: body}
}

func TestAttachingTheSameEvidenceTwiceKeepsOneCopy(t *testing.T) {
	dir := emptyBundle(t)
	for range 2 {
		if err := Attach(dir, []Evidence{sbom(t)}); err != nil {
			t.Fatalf("attach: %v", err)
		}
	}
	artifact, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(artifact.Evidence) != 1 {
		t.Errorf("got %d pieces of evidence, want one", len(artifact.Evidence))
	}
}

func TestAttachRefusesTwoFilesOfOneName(t *testing.T) {
	err := Attach(emptyBundle(t), []Evidence{{Name: "sbom.json", Data: []byte("a")}, {Name: "sbom.json", Data: []byte("b")}})
	if !errors.Is(err, ErrEvidenceSameName) {
		t.Fatalf("got %v, want ErrEvidenceSameName", err)
	}
}

func TestOpenRefusesTwoPiecesOfEvidenceOfOneName(t *testing.T) {
	dir := emptyBundle(t)
	root, err := bundleEntry(dir)
	if err != nil {
		t.Fatalf("bundleEntry: %v", err)
	}
	for _, data := range []string{"a", "b"} {
		if err := attach(dir, root, []Evidence{{Name: "sbom.json", Data: []byte(data)}}); err != nil {
			t.Fatalf("attach without checks: %v", err)
		}
	}
	if _, err := Open(dir); !errors.Is(err, ErrEvidenceTwice) {
		t.Fatalf("got %v, want ErrEvidenceTwice", err)
	}
}

func TestOpenSkipsATopLevelManifestWhoseBlobIsAbsent(t *testing.T) {
	dir := emptyBundle(t)
	indexFile, err := readIndex(dir)
	if err != nil {
		t.Fatalf("readIndex: %v", err)
	}
	indexFile.Manifests = append(indexFile.Manifests, entry{MediaType: manifestType,
		Digest: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Size: 2})
	if err := writeIndex(dir, indexFile); err != nil {
		t.Fatalf("writeIndex: %v", err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatalf("open: %v", err)
	}
}
