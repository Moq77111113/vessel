package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
)

func TestBuildPinsEveryComposeImageToADigest(t *testing.T) {
	source := unpackFixture(t, serveRegistry(t), "testdata/compose")
	out, exe := filepath.Join(t.TempDir(), "bundle"), filepath.Join(t.TempDir(), "acme")
	if err := runVessel("build", "-o", exe, "--layout", out, "--insecure-unsigned", source); err != nil {
		t.Fatalf("build: %v", err)
	}
	artifact, err := bundle.Open(out)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if artifact.Config.Machine != "compose" {
		t.Errorf("machine: got %q, want compose", artifact.Config.Machine)
	}
	body := string(artifact.Files[0].Data)
	if got := strings.Count(body, "@sha256:"); got != 2 {
		t.Errorf("got %d pinned images, want 2:\n%s", got, body)
	}
	if strings.Contains(body, ":1.0") || strings.Contains(body, ":17.2") {
		t.Errorf("still carries a tag:\n%s", body)
	}
}
