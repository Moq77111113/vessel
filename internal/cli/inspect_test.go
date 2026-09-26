package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
)

func TestInspectSaysAnInsecureBundleIsInsecure(t *testing.T) {
	var out bytes.Buffer
	if err := inspectBundle(&out, bundleWith(t, true)); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.Contains(out.String(), "insecure") {
		t.Errorf("got %q, want the bundle named insecure", out.String())
	}
}

func TestInspectSaysNothingAboutSigningForASignedBundle(t *testing.T) {
	var out bytes.Buffer
	if err := inspectBundle(&out, bundleWith(t, false)); err != nil {
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
