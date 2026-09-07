package descriptor

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// stub is a reader that recognizes a directory holding one named file.
type stub struct {
	name  string
	marks string
}

func (s stub) Name() string { return s.name }

func (s stub) Detect(dir fs.FS) bool {
	_, err := fs.Stat(dir, s.marks)
	return err == nil
}

func (s stub) Read(fs.FS) (Manifest, error) { return Manifest{}, nil }

func (s stub) Start([]File) []string { return nil }

func (s stub) Requires([]File) []string { return nil }

func (s stub) Owns(string) bool { return false }

func TestPickReturnsTheReaderThatRecognizesTheDirectory(t *testing.T) {
	readers := []Reader{stub{name: "compose", marks: "compose.yaml"}, stub{name: "quadlet", marks: "app.container"}}
	reader, err := Pick(readers, fstest.MapFS{"app.container": {}})
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got, want := reader.Name(), "quadlet"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPickKeepsTheOrderItWasGiven(t *testing.T) {
	both := fstest.MapFS{"compose.yaml": {}, "app.container": {}}
	readers := []Reader{stub{name: "compose", marks: "compose.yaml"}, stub{name: "quadlet", marks: "app.container"}}
	reader, err := Pick(readers, both)
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got, want := reader.Name(), "compose"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPickNamesTheFormatsItTriedWhenNoneMatch(t *testing.T) {
	readers := []Reader{stub{name: "compose", marks: "compose.yaml"}, stub{name: "quadlet", marks: "app.container"}}
	_, err := Pick(readers, fstest.MapFS{"README.md": {}})
	if !errors.Is(err, ErrNoDescriptor) {
		t.Fatalf("got %v, want ErrNoDescriptor", err)
	}
	for _, name := range []string{"compose", "quadlet"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error does not name %q: %v", name, err)
		}
	}
}

func TestByNameFindsTheReaderThatProducedABundle(t *testing.T) {
	readers := []Reader{stub{name: "compose"}, stub{name: "quadlet"}}
	reader, err := ByName(readers, "quadlet")
	if err != nil {
		t.Fatalf("ByName: %v", err)
	}
	if got, want := reader.Name(), "quadlet"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestByNameRefusesAFormatThisBuildCannotRead(t *testing.T) {
	if _, err := ByName([]Reader{stub{name: "quadlet"}}, "compose"); !errors.Is(err, ErrNoDescriptor) {
		t.Errorf("got %v, want ErrNoDescriptor", err)
	}
}

func TestPickWithoutAnyReaderFails(t *testing.T) {
	if _, err := Pick(nil, fstest.MapFS{}); !errors.Is(err, ErrNoDescriptor) {
		t.Errorf("got %v, want ErrNoDescriptor", err)
	}
}
