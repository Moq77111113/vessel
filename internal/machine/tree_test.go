package machine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	ok, err := NewTree(t.TempDir()).Write(unit())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !ok {
		t.Error("got false, want true on a first write")
	}
}

func TestTreeReportsNoChangeOnASecondIdenticalWrite(t *testing.T) {
	tree := NewTree(t.TempDir())
	if _, err := tree.Write(unit()); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	ok, err := tree.Write(unit())
	if err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if ok {
		t.Error("got true, want false on an identical write")
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
	ok, err := NewTree(root).Write(unit())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !ok {
		t.Error("got false, want true when the content differs")
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

func TestTreeRemoveDeletesTheFileUnderTheRoot(t *testing.T) {
	root := t.TempDir()
	tree := NewTree(root)
	if _, err := tree.Write(unit()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ok, err := tree.Remove(unit().Path)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !ok {
		t.Error("got false, want true on a file that was there")
	}
	if _, err := os.Stat(filepath.Join(root, unit().Path)); !os.IsNotExist(err) {
		t.Error("Remove left the file in place")
	}
}

func TestTreeRemoveIsANoOpOnAFileAlreadyGone(t *testing.T) {
	ok, err := NewTree(t.TempDir()).Remove("etc/containers/systemd/web.container")
	if err != nil {
		t.Errorf("Remove: %v, want nil on a file already gone", err)
	}
	if ok {
		t.Error("got true, want false on a file that was never there")
	}
}

func TestTreeRemoveRefusesAPathThatLeavesTheRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "escape.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := NewTree(root).Remove("../escape.txt")
	if !errors.Is(err, ErrPathEscapes) {
		t.Errorf("got %v, want ErrPathEscapes", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("Remove deleted a file outside the root")
	}
}

func TestTreeRemoveRefusesAPathThatNamesTheRootItself(t *testing.T) {
	for _, name := range []string{"", ".", "./"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			_, err := NewTree(root).Remove(name)
			if !errors.Is(err, ErrPathEscapes) {
				t.Errorf("got %v, want ErrPathEscapes", err)
			}
			if _, err := os.Stat(root); err != nil {
				t.Error("Remove deleted the install root")
			}
		})
	}
}

func TestTreeWriteRefusesAPathThatNamesTheRootItself(t *testing.T) {
	file := descriptor.File{Path: "", Data: []byte("x")}
	if _, err := NewTree(t.TempDir()).Write(file); !errors.Is(err, ErrPathEscapes) {
		t.Errorf("got %v, want ErrPathEscapes", err)
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

func TestTreeRefusesToWriteUnderADirectoryOthersCanWrite(t *testing.T) {
	root := t.TempDir()
	open := filepath.Join(root, "etc", "containers")
	if err := os.MkdirAll(open, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if _, err := NewTree(root).Write(unit()); !errors.Is(err, ErrUnsafeParent) {
		t.Fatalf("got %v, want ErrUnsafeParent", err)
	}
}

func TestTreeRefusesToWriteThroughALinkIntoADirectoryOthersCanWrite(t *testing.T) {
	root := t.TempDir()
	open := filepath.Join(root, "srv", "open")
	if err := os.MkdirAll(open, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if err := os.Symlink(open, filepath.Join(root, "etc")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := NewTree(root).Write(unit()); !errors.Is(err, ErrUnsafeParent) {
		t.Fatalf("got %v, want ErrUnsafeParent", err)
	}
}

func TestTreeWritesThroughALinkOnlyItsOwnerCouldPlace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "usr", "etc"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink("usr/etc", filepath.Join(root, "etc")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := NewTree(root).Write(unit()); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestTreeRefusesToRemoveUnderADirectoryOthersCanWrite(t *testing.T) {
	root := t.TempDir()
	if _, err := NewTree(root).Write(unit()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := os.Chmod(filepath.Join(root, "etc", "containers"), 0o777); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if _, err := NewTree(root).Remove(unit().Path); !errors.Is(err, ErrUnsafeParent) {
		t.Fatalf("got %v, want ErrUnsafeParent", err)
	}
}
