package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
)

func TestInspectSaysAnInsecureBundleIsInsecure(t *testing.T) {
	var out bytes.Buffer
	if err := inspectBundle(&out, bundleWith(t, true), ""); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.Contains(out.String(), "insecure") {
		t.Errorf("got %q, want the bundle named insecure", out.String())
	}
}

func TestInspectSaysNothingAboutSigningForASignedBundle(t *testing.T) {
	var out bytes.Buffer
	if err := inspectBundle(&out, bundleWith(t, false), ""); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if strings.Contains(out.String(), "insecure") {
		t.Errorf("got %q, want no insecure line", out.String())
	}
}

// bundleWith writes a bundle over testdata/empty-layout whose config carries insecure.
func bundleWith(t *testing.T, insecure bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := bundle.Contents{
		Layout: "testdata/empty-layout",
		Config: bundle.Config{Name: "acme", Version: "1.4.0", Machine: "quadlet", Platform: "linux/amd64", Insecure: insecure},
	}
	if err := bundle.Write(dir, contents); err != nil {
		t.Fatalf("write the bundle: %v", err)
	}
	return dir
}

func TestInspectListsTheEvidenceABundleCarries(t *testing.T) {
	var out bytes.Buffer
	if err := inspectBundle(&out, bundleWithSBOM(t), ""); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.Contains(out.String(), "sbom.spdx.json") {
		t.Errorf("got %q, want the SBOM listed", out.String())
	}
}

func TestInspectExtractsTheEvidenceIntoADirectory(t *testing.T) {
	into := t.TempDir()
	if err := inspectBundle(io.Discard, bundleWithSBOM(t), into); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(into, "sbom.spdx.json"))
	if err != nil {
		t.Fatalf("the SBOM never reached the directory: %v", err)
	}
	fixture, err := os.ReadFile("testdata/sbom.spdx.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	if !bytes.Equal(body, fixture) {
		t.Errorf("got %q, want %q", body, fixture)
	}
}

func TestInspectRefusesToOverwriteAFileWhileExtracting(t *testing.T) {
	into := t.TempDir()
	if err := os.WriteFile(filepath.Join(into, "sbom.spdx.json"), []byte("mine"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := inspectBundle(io.Discard, bundleWithSBOM(t), into); !errors.Is(err, os.ErrExist) {
		t.Fatalf("got %v, want os.ErrExist", err)
	}
	body, err := os.ReadFile(filepath.Join(into, "sbom.spdx.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "mine" {
		t.Errorf("got %q, want the operator's file left alone", body)
	}
}

// bundleWithSBOM writes a signed-config bundle carrying testdata/sbom.spdx.json.
func bundleWithSBOM(t *testing.T) string {
	t.Helper()
	dir := bundleWith(t, false)
	body, err := os.ReadFile("testdata/sbom.spdx.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	if err := bundle.Attach(dir, []bundle.Evidence{{Name: "sbom.spdx.json", Data: body}}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	return dir
}

func TestInspectWritesNothingWhenOneTargetIsTaken(t *testing.T) {
	dir := bundleWithSBOM(t)
	if err := bundle.Attach(dir, []bundle.Evidence{{Name: "trivy.json", Data: []byte("{}")}}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	into := t.TempDir()
	if err := os.WriteFile(filepath.Join(into, "trivy.json"), []byte("mine"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := inspectBundle(io.Discard, dir, into); !errors.Is(err, os.ErrExist) {
		t.Fatalf("got %v, want os.ErrExist", err)
	}
	if _, err := os.Stat(filepath.Join(into, "sbom.spdx.json")); !os.IsNotExist(err) {
		t.Errorf("the SBOM was written although another target was taken, err=%v", err)
	}
}

func TestInspectCreatesTheDirectoryItExtractsInto(t *testing.T) {
	into := filepath.Join(t.TempDir(), "audit")
	if err := inspectBundle(io.Discard, bundleWithSBOM(t), into); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if _, err := os.Stat(filepath.Join(into, "sbom.spdx.json")); err != nil {
		t.Errorf("the SBOM never reached a new directory: %v", err)
	}
}
