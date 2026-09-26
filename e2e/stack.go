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
// Both are built on one layer layer, so the bundle has something to deduplicate.
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
	layer, err := random.Layer(256, types.OCILayer)
	if err != nil {
		t.Fatalf("random.Layer: %v", err)
	}
	for _, repository := range images {
		image, err := randomOCIImage(layer)
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
func randomOCIImage(layer v1.Layer) (v1.Image, error) {
	own, err := random.Layer(128, types.OCILayer)
	if err != nil {
		return nil, err
	}
	base := mutate.ConfigMediaType(mutate.MediaType(empty.Image, types.OCIManifestSchema1), types.OCIConfigJSON)
	return mutate.AppendLayers(base, own, layer)
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

// buildStack builds the fixture stack and returns the bundle directory.
func buildStack(t *testing.T) string {
	t.Helper()
	out, exe := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "vessel-stack")
	args := []string{"build", "-o", exe, "--layout", out, "--name", "acme", "--version", "1.0", "--insecure-unsigned", serveStack(t)}
	if err := runVessel(args...); err != nil {
		t.Fatalf("build: %v", err)
	}
	return out
}

// buildDelivery lays extra files into the fixture stack, builds it, and returns the bundle
// directory. A test asserting on build's refusal calls buildDeliveryErr instead.
func buildDelivery(t *testing.T, files map[string]string) string {
	t.Helper()
	out, err := buildFixture(t, files)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return out
}

// buildDeliveryErr does the same and returns build's error instead of failing on it.
func buildDeliveryErr(t *testing.T, files map[string]string) error {
	t.Helper()
	_, err := buildFixture(t, files)
	return err
}

func buildFixture(t *testing.T, files map[string]string) (string, error) {
	t.Helper()
	source := serveStack(t)
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	out, exe := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "vessel-stack")
	return out, runVessel("build", "-o", exe, "--layout", out, "--insecure-unsigned", source)
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
