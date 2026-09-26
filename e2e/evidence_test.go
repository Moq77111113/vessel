package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvidenceMadeFromALayoutTravelsWithTheExecutable(t *testing.T) {
	dir := t.TempDir()
	layout := filepath.Join(dir, "bundle")
	if err := runVessel("build", "-o", filepath.Join(dir, "scratch"), "--layout", layout,
		"--name", "acme", "--version", "1.0", "--insecure-unsigned", serveStack(t)); err != nil {
		t.Fatalf("first build: %v", err)
	}
	if err := runVessel("build", "-o", filepath.Join(dir, "myapp"), "--insecure-unsigned",
		"--evidence", "testdata/evidence/sbom.spdx.json", layout); err != nil {
		t.Fatalf("build from the layout: %v", err)
	}
	output, err := runVesselCapture("inspect", layout)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.Contains(output, "sbom.spdx.json") {
		t.Errorf("got %q, want the SBOM listed", output)
	}
	into := t.TempDir()
	if err := runVessel("inspect", "--evidence", into, layout); err != nil {
		t.Fatalf("extract: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(into, "sbom.spdx.json"))
	if err != nil {
		t.Fatalf("the SBOM never reached the directory: %v", err)
	}
	fixture, err := os.ReadFile("testdata/evidence/sbom.spdx.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	if !bytes.Equal(body, fixture) {
		t.Errorf("got %q, want the fixture", body)
	}
}
