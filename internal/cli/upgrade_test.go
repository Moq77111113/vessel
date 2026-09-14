package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
)

func TestOnlyInNamesAFileThePreviousVersionCarriedThatTheNewOneDoesNot(t *testing.T) {
	previous := []machine.Entry{
		{Path: "etc/containers/systemd/web.container", Digest: "sha256:aaaa"},
		{Path: "etc/containers/systemd/cache.container", Digest: "sha256:bbbb"},
	}
	next := []machine.Entry{
		{Path: "etc/containers/systemd/web.container", Digest: "sha256:cccc"},
	}
	got := onlyIn(previous, next)
	want := []string{"etc/containers/systemd/cache.container"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestOnlyInKeepsAFileBothVersionsCarry(t *testing.T) {
	entries := []machine.Entry{{Path: "etc/acme/realm.json", Digest: "sha256:aaaa"}}
	if got := onlyIn(entries, entries); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

// TestUpgradeRemovesAFileTheNewVersionDoesNotCarry installs a delivery carrying two files, then a
// newer version carrying only one of them, and checks the dropped file is gone while the kept one
// reflects the new version.
func TestUpgradeRemovesAFileTheNewVersionDoesNotCarry(t *testing.T) {
	root := t.TempDir()
	first := linkTestBundle(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.3.0
files:
  - source: a.txt
    target: /etc/acme/a.txt
  - source: b.txt
    target: /etc/acme/b.txt
`,
		"a.txt": "old-a",
		"b.txt": "old-b",
	})
	if err := install(t, root, first, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}

	second := linkTestBundle(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.4.0
files:
  - source: b.txt
    target: /etc/acme/b.txt
`,
		"b.txt": "new-b",
	})
	if err := install(t, root, second, nil); err != nil {
		t.Fatalf("install 1.4.0: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "etc/acme/a.txt")); !os.IsNotExist(err) {
		t.Errorf("a.txt is still on disk after the upgrade dropped it, err=%v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "etc/acme/b.txt"))
	if err != nil {
		t.Fatalf("b.txt is missing after the upgrade: %v", err)
	}
	if string(body) != "new-b" {
		t.Errorf("got %q, want %q", string(body), "new-b")
	}
}

func TestInstallOnAFreshMachinePutsEveryFileOnDisk(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.0.0
files:
  - source: a.txt
    target: /etc/acme/a.txt
  - source: b.txt
    target: /etc/acme/b.txt
`,
		"a.txt": "A",
		"b.txt": "B",
	})
	if err := install(t, root, dir, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	for _, path := range []string{
		"etc/acme/a.txt", "etc/acme/b.txt", "etc/containers/systemd/web.container",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Errorf("%s is missing after a fresh install: %v", path, err)
		}
	}
}

func TestUpgradeRefusesAMachineWithNoRecord(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	err := upgrade(t, root, dir, nil)
	if !errors.Is(err, ErrNoRecord) {
		t.Errorf("got %v, want ErrNoRecord", err)
	}
}

// TestUpgradeStopsTheServiceOfAUnitItDrops installs a delivery carrying two units, then a version
// dropping one of them, and checks the dropped unit's service is stopped, not just its file removed.
func TestUpgradeStopsTheServiceOfAUnitItDrops(t *testing.T) {
	root := t.TempDir()
	first := linkTestBundle(t, map[string]string{
		"vessel.yaml":     "name: acme\nversion: 1.3.0\n",
		"cache.container": "[Container]\nContainerName=cache\n",
	})
	if err := install(t, root, first, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}

	second := linkTestBundle(t, map[string]string{"vessel.yaml": "name: acme\nversion: 1.4.0\n"})
	stub := &podmanStub{}
	if err := upgrade(t, root, second, []machine.Machine{quadlet.New(stub.run)}); err != nil {
		t.Fatalf("upgrade 1.4.0: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl stop cache.service") {
		t.Errorf("upgrade never stopped cache.service: %v", stub.calls)
	}
}

// TestUpgradeDoesNotReportAFileAlreadyGoneByHand checks the Removing line is gated on the file
// actually having been there, not printed just because the record still names it.
func TestUpgradeDoesNotReportAFileAlreadyGoneByHand(t *testing.T) {
	root := t.TempDir()
	first := linkTestBundle(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.3.0
files:
  - source: a.txt
    target: /etc/acme/a.txt
`,
		"a.txt": "old-a",
	})
	if err := install(t, root, first, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "etc/acme/a.txt")); err != nil {
		t.Fatalf("remove a.txt by hand: %v", err)
	}

	second := linkTestBundle(t, map[string]string{"vessel.yaml": "name: acme\nversion: 1.4.0\n"})
	var out bytes.Buffer
	err := load(context.Background(), io.Discard, report.New(&out), strings.NewReader(""),
		[]machine.Machine{quadlet.New((&podmanStub{}).run)}, machine.NewShell(noCapture),
		second, root, nil, false, modeUpgrade)
	if err != nil {
		t.Fatalf("upgrade 1.4.0: %v", err)
	}
	if strings.Contains(out.String(), "Removing") {
		t.Errorf("got %q, want no Removing line for a file already gone", out.String())
	}
}

// TestAFailedActionLeavesAStaleFileInPlace checks an upgrade that dies on its action never
// removes a dropped file: the machine keeps holding a superset of a working configuration.
func TestAFailedActionLeavesAStaleFileInPlace(t *testing.T) {
	root := t.TempDir()
	first := linkTestBundle(t, map[string]string{
		"vessel.yaml": `
name: acme
version: 1.3.0
files:
  - source: a.txt
    target: /etc/acme/a.txt
`,
		"a.txt": "old-a",
	})
	if err := install(t, root, first, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}

	second := linkTestBundle(t, map[string]string{
		"vessel.yaml": "name: acme\nversion: 1.4.0\nactions:\n  - echo hi\n",
	})
	if err := upgrade(t, root, second, nil); err == nil {
		t.Fatal("upgrade succeeded despite a shell that refuses every action")
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/a.txt")); err != nil {
		t.Errorf("a.txt was removed before the failing action ran: %v", err)
	}
}
