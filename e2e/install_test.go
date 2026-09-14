package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
)

func TestAQuadletStackReachesACleanRootFromABundle(t *testing.T) {
	needPodman(t)
	takeSystemctl(t)

	root := t.TempDir()
	if err := runVessel("install", "--root", root, buildStack(t)); err != nil {
		t.Fatalf("install: %v", err)
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

func TestInstallingTwiceChangesNothingTheSecondTime(t *testing.T) {
	needPodman(t)
	takeSystemctl(t)

	root, bundle := t.TempDir(), buildStack(t)
	if err := runVessel("install", "--root", root, bundle); err != nil {
		t.Fatalf("first install: %v", err)
	}
	unit := filepath.Join(root, "etc/containers/systemd/web.container")
	before, err := os.Stat(unit)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if err := runVessel("install", "--root", root, bundle); err != nil {
		t.Fatalf("second install: %v", err)
	}
	after, err := os.Stat(unit)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("the second install rewrote a file whose content had not changed")
	}
}

func TestInstallPutsTheValueTheOperatorGaveIntoTheFile(t *testing.T) {
	needPodman(t)
	takeSystemctl(t)

	dir := buildDelivery(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.4.0
files:
  - source: realm.json
    target: /etc/acme/realm.json
variables:
  - name: PUBLIC_HOST
    description: public address
`,
		"realm.json": `{"realm":"###PUBLIC_HOST###"}`,
	})
	root := t.TempDir()
	if err := runVessel("install", "--root", root, "--set", "PUBLIC_HOST=dmas.acme.local", dir); err != nil {
		t.Fatalf("install: %v", err)
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
	takeSystemctl(t)

	dir := buildDelivery(t, map[string]string{"vessel.yaml": "name: acme\nvariables:\n  - name: PUBLIC_HOST\n"})
	root := t.TempDir()
	if err := runVessel("install", "--root", root, dir); err == nil {
		t.Fatal("install succeeded with no value for PUBLIC_HOST")
	}
	if _, err := os.Stat(filepath.Join(root, "etc/containers/systemd")); err == nil {
		t.Error("install wrote units even though a value was missing")
	}
}

// needPodman skips the test unless podman is here, new enough, and able to start.
func needPodman(t *testing.T) {
	t.Helper()
	if err := quadlet.New(machine.Exec).Check(context.Background(), t.TempDir()); err != nil {
		t.Skipf("this machine cannot run a quadlet stack: %v", err)
	}
}

// takeSystemctl puts a systemctl on PATH that records its arguments, and returns that record.
// Install ends by starting what it wrote, and units under a temporary root are units the
// machine's own systemd never sees.
func takeSystemctl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "calls")
	script := "#!/bin/sh\necho \"$@\" >>" + record + "\n" +
		"if [ \"$1\" = is-active ]; then echo active; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return record
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
