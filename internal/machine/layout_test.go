package machine_test

import (
	"archive/tar"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/machine/layouttest"
)

func TestLayoutNamesEveryImageItHolds(t *testing.T) {
	names, err := layouttest.Open(t).Names()
	if err != nil {
		t.Fatalf("Names: %v", err)
	}
	want := []string{"registry.example.com/acme/web:1.0", "registry.example.com/library/postgres:17.2"}
	if len(names) != len(want) {
		t.Fatalf("names: got %d, want %d", len(names), len(want))
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("name %d: got %q, want %q", i, names[i], want[i])
		}
	}
}

func TestArchiveHoldsOneManifestAndItsBlobs(t *testing.T) {
	var out strings.Builder
	if err := layouttest.Open(t).Archive(0, &out); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	names := entryNames(t, out.String())
	if got, want := countBlobs(names), 3; got != want {
		t.Errorf("blobs: got %d, want %d (manifest, config, layer)", got, want)
	}
	for _, needed := range []string{"index.json", "oci-layout"} {
		if !names[needed] {
			t.Errorf("archive misses %s", needed)
		}
	}
}

func TestArchiveIndexNamesOnlyTheImageItCarries(t *testing.T) {
	var out strings.Builder
	if err := layouttest.Open(t).Archive(1, &out); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	index := entryBody(t, out.String(), "index.json")
	if !strings.Contains(index, "library/postgres:17.2") {
		t.Errorf("index does not name the image: %s", index)
	}
	if strings.Contains(index, "acme/web:1.0") {
		t.Errorf("index carries the other image too: %s", index)
	}
}

func TestOpenLayoutRefusesABlobThatDoesNotMatchItsDigest(t *testing.T) {
	dir := layouttest.TwoImages(t)
	entries, err := os.ReadDir(filepath.Join(dir, "blobs", "sha256"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	path := filepath.Join(dir, "blobs", "sha256", entries[0].Name())
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	layout, err := machine.OpenLayout(dir)
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	var out strings.Builder
	err = errors.Join(layout.Archive(0, &out), layout.Archive(1, &out))
	if !errors.Is(err, machine.ErrBlobDigest) {
		t.Errorf("got %v, want ErrBlobDigest", err)
	}
}

func entryNames(t *testing.T, body string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	archive := tar.NewReader(strings.NewReader(body))
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatalf("read the archive: %v", err)
		}
		names[header.Name] = true
	}
}

func entryBody(t *testing.T, body, want string) string {
	t.Helper()
	archive := tar.NewReader(strings.NewReader(body))
	for {
		header, err := archive.Next()
		if err == io.EOF {
			t.Fatalf("no %s in the archive", want)
		}
		if err != nil {
			t.Fatalf("read the archive: %v", err)
		}
		if header.Name == want {
			data, err := io.ReadAll(archive)
			if err != nil {
				t.Fatalf("read %s: %v", want, err)
			}
			return string(data)
		}
	}
}

func countBlobs(names map[string]bool) int {
	count := 0
	for name := range names {
		if strings.HasPrefix(name, "blobs/sha256/") && !strings.HasSuffix(name, "/") {
			count++
		}
	}
	return count
}
