package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildVessel compiles the binary the operator would receive.
func buildVessel(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vessel")
	build := exec.Command("go", "build", "-o", path, "../cmd/vessel")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v: %s", err, output)
	}
	return path
}

// packStack builds the fixture stack into one executable.
func packStack(t *testing.T) string {
	t.Helper()
	vessel, out := buildVessel(t), filepath.Join(t.TempDir(), "myapp")
	build := exec.Command(vessel, "build", serveStack(t), "-o", out, "--name", "acme", "--version", "1.0")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("vessel build: %v: %s", err, output)
	}
	return out
}

func TestPackWritesAnExecutable(t *testing.T) {
	info, err := os.Stat(packStack(t))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("mode %v, want it executable", info.Mode())
	}
}

func TestAPackedFileOffersInstallUpgradeAndNothingToBuild(t *testing.T) {
	output, err := exec.Command(packStack(t), "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help: %v: %s", err, output)
	}
	for _, verb := range []string{"inspect", "install", "upgrade"} {
		if !strings.Contains(string(output), verb) {
			t.Errorf("the packed file does not offer %q: %s", verb, output)
		}
	}
	for _, verb := range []string{"build", "link", "pack"} {
		if strings.Contains(string(output), verb) {
			t.Errorf("the packed file still offers %q, it should not: %s", verb, output)
		}
	}
}

func TestAPackedFileInspectsWhatItCarries(t *testing.T) {
	output, err := exec.Command(packStack(t), "inspect").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect: %v: %s", err, output)
	}
	for _, want := range []string{"acme 1.0", "web.container", "sha256:"} {
		if !strings.Contains(string(output), want) {
			t.Errorf("inspect does not mention %q: %s", want, output)
		}
	}
}
