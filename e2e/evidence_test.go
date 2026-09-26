package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sbom = "testdata/evidence/sbom.spdx.json"

func TestInspectListsTheEvidenceALayoutCarries(t *testing.T) {
	vessel, layout, _ := buildWithEvidence(t)
	output, err := exec.Command(vessel, "inspect", layout).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect: %v: %s", err, output)
	}
	if !strings.Contains(string(output), filepath.Base(sbom)) {
		t.Errorf("got %q, want the SBOM listed", output)
	}
}

func TestAPackedFileExtractsTheEvidenceItCarries(t *testing.T) {
	_, _, myapp := buildWithEvidence(t)
	into := t.TempDir()
	if output, err := exec.Command(myapp, "inspect", "--evidence", into).CombinedOutput(); err != nil {
		t.Fatalf("inspect --evidence: %v: %s", err, output)
	}
	body, err := os.ReadFile(filepath.Join(into, filepath.Base(sbom)))
	if err != nil {
		t.Fatalf("the SBOM never reached the directory: %v", err)
	}
	fixture, err := os.ReadFile(sbom)
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	if !bytes.Equal(body, fixture) {
		t.Errorf("got %q, want the fixture", body)
	}
}

// buildWithEvidence packs the fixture stack with the SBOM, returning vessel, the layout and the executable.
func buildWithEvidence(t *testing.T) (string, string, string) {
	t.Helper()
	vessel, dir := buildVessel(t), t.TempDir()
	layout, myapp := filepath.Join(dir, "bundle"), filepath.Join(dir, "myapp")
	for _, args := range [][]string{
		{"build", "-o", filepath.Join(dir, "scratch"), "--layout", layout, "--name", "acme", "--version", "1.0", "--insecure-unsigned", serveStack(t)},
		{"build", "-o", myapp, "--insecure-unsigned", "--evidence", sbom, layout},
	} {
		if output, err := exec.Command(vessel, args...).CombinedOutput(); err != nil {
			t.Fatalf("vessel %v: %v: %s", args, err, output)
		}
	}
	return vessel, layout, myapp
}
