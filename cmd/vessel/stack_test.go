package main

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

	"github.com/Moq77111113/vessel/pkg/delivery"
)

const fixtureHost = "registry.test"

var images = []string{"acme/web:1.0", "library/postgres:17.2"}

const multiPlatform = "library/postgres:17.2"

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

func randomOCIImage(layer v1.Layer) (v1.Image, error) {
	own, err := random.Layer(128, types.OCILayer)
	if err != nil {
		return nil, err
	}
	base := mutate.ConfigMediaType(mutate.MediaType(empty.Image, types.OCIManifestSchema1), types.OCIConfigJSON)
	return mutate.AppendLayers(base, own, layer)
}

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

func buildStack(t *testing.T) string {
	t.Helper()
	out, exe := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "vessel-stack")
	args := []string{"build", "-o", exe, "--layout", out, "--name", "acme", "--version", "1.0", "--insecure-unsigned", serveStack(t)}
	if err := runVessel(args...); err != nil {
		t.Fatalf("build: %v", err)
	}
	return out
}

func buildDelivery(t *testing.T, files map[string]string) string {
	t.Helper()
	out, err := buildFixture(t, files)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return out
}

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

func paths(files []delivery.File) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Path
	}
	return names
}

func runVessel(args ...string) error {
	vessel := New()
	vessel.SetArgs(args)
	vessel.SetOut(io.Discard)
	vessel.SetErr(io.Discard)
	return vessel.Execute()
}

func runVesselCapture(args ...string) (string, error) {
	var out bytes.Buffer
	vessel := New()
	vessel.SetArgs(args)
	vessel.SetOut(&out)
	vessel.SetErr(&out)
	err := vessel.Execute()
	return out.String(), err
}
