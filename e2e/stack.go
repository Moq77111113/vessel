// Package e2e drives the whole vessel command line against a real registry.
package e2e

import (
	"bytes"
	"io"
	"io/fs"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	ggcr "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/Moq77111113/vessel/internal/cli"
	"github.com/Moq77111113/vessel/internal/descriptor"
)

// fixtureHost is the registry the units in testdata point at, swapped for the live one.
const fixtureHost = "registry.test"

// images is what the fixture units ask for, and what the registry is filled with.
// Both are built on one shared layer, so the bundle has something to deduplicate.
var images = []string{"acme/web:1.0", "library/postgres:17.2"}

// multiPlatform is served behind a single-platform index rather than directly: real
// multi-arch images ship that way, and a bundle once pinned the index digest instead
// of the platform manifest the bundle actually carried, which podman refused to load.
const multiPlatform = "library/postgres:17.2"

// serveStack fills a registry with the images and lays the fixture units down pointing at it.
func serveStack(t *testing.T) string {
	t.Helper()
	return unpackFixture(t, serveRegistry(t))
}

func serveRegistry(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(ggcr.New())
	t.Cleanup(server.Close)

	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	shared, err := random.Layer(256, types.OCILayer)
	if err != nil {
		t.Fatalf("random.Layer: %v", err)
	}
	for _, repository := range images {
		image, err := randomOCIImage(shared)
		if err != nil {
			t.Fatalf("randomOCIImage: %v", err)
		}
		tag, err := name.NewTag(address.Host + "/" + repository)
		if err != nil {
			t.Fatalf("NewTag: %v", err)
		}
		if repository == multiPlatform {
			index := mutate.AppendManifests(empty.Index, mutate.IndexAddendum{Add: image})
			if err := remote.WriteIndex(tag, index); err != nil {
				t.Fatalf("remote.WriteIndex: %v", err)
			}
			continue
		}
		if err := remote.Write(tag, image); err != nil {
			t.Fatalf("remote.Write: %v", err)
		}
	}
	return address.Host
}

// randomOCIImage builds a pseudo-random image whose manifest, config and layers all
// carry OCI media types. random.Image mixes docker media types into an OCI manifest,
// which podman refuses to load: a bundle needs one coherent format, not a hybrid.
func randomOCIImage(shared v1.Layer) (v1.Image, error) {
	own, err := random.Layer(128, types.OCILayer)
	if err != nil {
		return nil, err
	}
	base := mutate.ConfigMediaType(mutate.MediaType(empty.Image, types.OCIManifestSchema1), types.OCIConfigJSON)
	return mutate.AppendLayers(base, own, shared)
}

// unpackFixture copies testdata/stack into a temporary directory, pointing it at host.
func unpackFixture(t *testing.T, host string) string {
	t.Helper()
	source, fixture := t.TempDir(), os.DirFS("testdata/stack")
	entries, err := fs.ReadDir(fixture, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		data, err := fs.ReadFile(fixture, entry.Name())
		if err != nil {
			t.Fatalf("ReadFile %s: %v", entry.Name(), err)
		}
		data = bytes.ReplaceAll(data, []byte(fixtureHost), []byte(host))
		if err := os.WriteFile(filepath.Join(source, entry.Name()), data, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", entry.Name(), err)
		}
	}
	return source
}

// linkStack links the fixture stack and returns the bundle directory.
func linkStack(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "bundle")
	args := []string{"link", "-o", out, "--name", "acme", "--version", "1.0", serveStack(t)}
	if err := runVessel(args...); err != nil {
		t.Fatalf("link: %v", err)
	}
	return out
}

// linkDelivery lays extra files into the fixture stack, links it, and returns the bundle
// directory. A test asserting on link's refusal calls linkDeliveryErr instead.
func linkDelivery(t *testing.T, files map[string]string) string {
	t.Helper()
	out, err := linkFixture(t, files)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	return out
}

// linkDeliveryErr does the same and returns link's error instead of failing on it.
func linkDeliveryErr(t *testing.T, files map[string]string) error {
	t.Helper()
	_, err := linkFixture(t, files)
	return err
}

func linkFixture(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	source := serveStack(t)
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	out := filepath.Join(t.TempDir(), "bundle")
	return out, runVessel("link", "-o", out, source)
}

// paths names the files a bundle carries, for a failure message.
func paths(files []descriptor.File) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Path
	}
	return names
}

// runVessel runs the command line with its output thrown away.
func runVessel(args ...string) error {
	vessel := cli.New()
	vessel.SetArgs(args)
	vessel.SetOut(io.Discard)
	vessel.SetErr(io.Discard)
	return vessel.Execute()
}

// runVesselCapture runs the command line and returns what it printed.
func runVesselCapture(args ...string) (string, error) {
	var out bytes.Buffer
	vessel := cli.New()
	vessel.SetArgs(args)
	vessel.SetOut(&out)
	vessel.SetErr(&out)
	err := vessel.Execute()
	return out.String(), err
}
