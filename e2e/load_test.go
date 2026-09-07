package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/target"
)

func TestAQuadletStackReachesACleanRootFromABundle(t *testing.T) {
	needPodman(t)

	root := t.TempDir()
	if err := runVessel("load", "--root", root, linkStack(t)); err != nil {
		t.Fatalf("load: %v", err)
	}
	unit, err := os.ReadFile(filepath.Join(root, "etc/containers/systemd/web.container"))
	if err != nil {
		t.Fatalf("the unit never reached the root: %v", err)
	}
	image := imageOf(t, string(unit))
	if err := exec.Command("podman", "image", "exists", image).Run(); err != nil {
		t.Errorf("podman cannot find %s in local storage: %v", image, err)
	}
}

func TestLoadingTwiceChangesNothingTheSecondTime(t *testing.T) {
	needPodman(t)

	root, bundle := t.TempDir(), linkStack(t)
	if err := runVessel("load", "--root", root, bundle); err != nil {
		t.Fatalf("first load: %v", err)
	}
	unit := filepath.Join(root, "etc/containers/systemd/web.container")
	before, err := os.Stat(unit)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if err := runVessel("load", "--root", root, bundle); err != nil {
		t.Fatalf("second load: %v", err)
	}
	after, err := os.Stat(unit)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("the second load rewrote a file whose content had not changed")
	}
}

func TestInstallPutsTheValueTheOperatorGaveIntoTheFile(t *testing.T) {
	needPodman(t)

	dir := linkDelivery(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.4.0
files:
  - source: realm.json
    target: /etc/acme/realm.json
variables:
  - name: PUBLIC_HOST
    ask: public address
`,
		"realm.json": `{"realm":"###PUBLIC_HOST###"}`,
	})
	root := t.TempDir()
	if err := runVessel("load", "--root", root, "--set", "PUBLIC_HOST=dmas.acme.local", dir); err != nil {
		t.Fatalf("load: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "etc/acme/realm.json"))
	if err != nil {
		t.Fatalf("the file never reached the root: %v", err)
	}
	if got, want := string(body), `{"realm":"dmas.acme.local"}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestInstallWritesNothingWhenAValueIsMissing(t *testing.T) {
	needPodman(t)

	dir := linkDelivery(t, map[string]string{"vessel.yaml": "name: acme\nvariables:\n  - name: PUBLIC_HOST\n"})
	root := t.TempDir()
	if err := runVessel("load", "--root", root, dir); err == nil {
		t.Fatal("load succeeded with no value for PUBLIC_HOST")
	}
	if _, err := os.Stat(filepath.Join(root, "etc/containers/systemd")); err == nil {
		t.Error("load wrote units even though a value was missing")
	}
}

// needPodman skips the test unless podman is here, new enough, and able to start.
func needPodman(t *testing.T) {
	t.Helper()
	version, err := target.NewLoader(target.Exec).Version(context.Background())
	if err != nil {
		t.Skipf("no podman on this machine: %v", err)
	}
	if err := target.CheckPodman(version); err != nil {
		t.Skipf("podman %s, this test needs 5.0 or newer", version)
	}
	if output, err := exec.Command("podman", "image", "ls").CombinedOutput(); err != nil {
		t.Skipf("podman %s cannot run here: %s", version, strings.TrimSpace(string(output)))
	}
}

func imageOf(t *testing.T, unit string) string {
	t.Helper()
	for line := range strings.SplitSeq(unit, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "Image="); ok {
			return value
		}
	}
	t.Fatalf("no Image= key in %q", unit)
	return ""
}
