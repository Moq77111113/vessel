package machine

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

func unit() descriptor.File {
	return descriptor.File{
		Path: "etc/containers/systemd/web.container",
		Data: []byte("[Container]\nImage=registry.example.com/acme/web@sha256:aaa\n"),
	}
}

func TestTreeWritesTheFileUnderTheRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := NewTree(root).Write(unit()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "etc/containers/systemd/web.container"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got, want := string(data), string(unit().Data); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTreeReportsAChangeOnTheFirstWrite(t *testing.T) {
	changed, err := NewTree(t.TempDir()).Write(unit())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !changed {
		t.Error("changed: got false, want true on a first write")
	}
}

func TestTreeReportsNoChangeOnASecondIdenticalWrite(t *testing.T) {
	tree := NewTree(t.TempDir())
	if _, err := tree.Write(unit()); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	changed, err := tree.Write(unit())
	if err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if changed {
		t.Error("changed: got true, want false on an identical write")
	}
}

func TestTreeOverwritesAFileTheOperatorEdited(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc/containers/systemd/web.container")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	changed, err := NewTree(root).Write(unit())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !changed {
		t.Error("changed: got false, want true when the content differs")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got, want := string(data), string(unit().Data); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTreeLeavesNoTemporaryFileBehind(t *testing.T) {
	root := t.TempDir()
	if _, err := NewTree(root).Write(unit()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "etc/containers/systemd"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if got, want := len(entries), 1; got != want {
		t.Errorf("entries: got %d, want %d", got, want)
	}
}

func TestTreeRefusesAPathThatLeavesTheRoot(t *testing.T) {
	file := descriptor.File{Path: "../escaped.container", Data: []byte("x")}
	if _, err := NewTree(t.TempDir()).Write(file); err == nil {
		t.Fatal("Write: want an error on a path leaving the root, got nil")
	}
}

func TestSameRefusesAFileEditedSinceItWasWritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "web.container")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("first"))
	digest := "sha256:" + hex.EncodeToString(sum[:])

	if !Same(path, digest) {
		t.Error("an untouched file does not match its digest")
	}
	if err := os.WriteFile(path, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Same(path, digest) {
		t.Error("an edited file still matches its digest")
	}
}

func TestSameRefusesAFileThatIsGone(t *testing.T) {
	if Same(filepath.Join(t.TempDir(), "absent"), "sha256:0000") {
		t.Error("a missing file matches a digest")
	}
}
