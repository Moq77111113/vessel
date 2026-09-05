package quadlet

import (
	"os"
	"strings"
	"testing"
)

func TestReaderDetectsADirectoryHoldingContainerUnits(t *testing.T) {
	if !NewReader().Detect(os.DirFS("testdata/stack")) {
		t.Fatal("Detect: want true on a quadlet directory")
	}
}

func TestReaderIgnoresADirectoryWithoutContainerUnits(t *testing.T) {
	if NewReader().Detect(os.DirFS(t.TempDir())) {
		t.Fatal("Detect: want false on a directory holding no unit")
	}
}

func TestReaderCarriesEveryUnitFileUnderTheSystemdPath(t *testing.T) {
	manifest, err := NewReader().Read(os.DirFS("testdata/stack"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := map[string]bool{
		"etc/containers/systemd/web.container": true,
		"etc/containers/systemd/db.container":  true,
		"etc/containers/systemd/app.network":   true,
	}
	if len(manifest.Files) != len(want) {
		t.Fatalf("files: got %d, want %d", len(manifest.Files), len(want))
	}
	for _, file := range manifest.Files {
		if !want[file.Path] {
			t.Errorf("unexpected path %q", file.Path)
		}
	}
}

func TestReaderLeavesOutAFileThatIsNotAUnit(t *testing.T) {
	manifest, err := NewReader().Read(os.DirFS("testdata/stack"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, file := range manifest.Files {
		if strings.HasSuffix(file.Path, "README.txt") {
			t.Errorf("README.txt should stay out of the bundle")
		}
	}
}

func TestReaderFindsOneRelocationPerImageKey(t *testing.T) {
	manifest, err := NewReader().Read(os.DirFS("testdata/stack"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got, want := len(manifest.Relocs), 2; got != want {
		t.Fatalf("relocations: got %d, want %d", got, want)
	}
}

func TestReaderSkipsACommentedImageKey(t *testing.T) {
	manifest, err := NewReader().Read(os.DirFS("testdata/stack"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, reloc := range manifest.Relocs {
		if strings.Contains(reloc.Ref.Tag, "16.4") {
			t.Errorf("the commented Image= line produced a relocation: %+v", reloc)
		}
	}
}

func TestRelocationSpansExactlyTheReferenceBytes(t *testing.T) {
	manifest, err := NewReader().Read(os.DirFS("testdata/stack"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	for _, reloc := range manifest.Relocs {
		data := manifest.Files[reloc.File].Data
		got := string(data[reloc.Offset : reloc.Offset+reloc.Length])
		if got != reloc.Ref.String() {
			t.Errorf("span: got %q, want %q", got, reloc.Ref.String())
		}
	}
}

func TestReadRejectsADirectoryWithNoUnitAtAll(t *testing.T) {
	if _, err := NewReader().Read(os.DirFS(t.TempDir())); err == nil {
		t.Fatal("Read: want an error on an empty directory, got nil")
	}
}
