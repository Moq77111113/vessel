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
		image, err := random.Image(128, 1)
		if err != nil {
			t.Fatalf("random.Image: %v", err)
		}
		image, err = mutate.AppendLayers(image, shared)
		if err != nil {
			t.Fatalf("AppendLayers: %v", err)
		}
		tag, err := name.NewTag(address.Host + "/" + repository)
		if err != nil {
			t.Fatalf("NewTag: %v", err)
		}
		if err := remote.Write(tag, image); err != nil {
			t.Fatalf("remote.Write: %v", err)
		}
	}
	return address.Host
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

// linkDelivery lays extra files into the fixture stack, links it, and returns the bundle directory.
func linkDelivery(t *testing.T, files map[string]string) string {
	t.Helper()
	source := serveStack(t)
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	out := filepath.Join(t.TempDir(), "bundle")
	if err := runVessel("link", "-o", out, source); err != nil {
		t.Fatalf("link: %v", err)
	}
	return out
}

// linkDeliveryErr lays extra files into the fixture stack and links it, returning link's error.
func linkDeliveryErr(t *testing.T, files map[string]string) error {
	t.Helper()
	source := serveStack(t)
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	out := filepath.Join(t.TempDir(), "bundle")
	return runVessel("link", "-o", out, source)
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
