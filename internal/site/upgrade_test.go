package site

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

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/driver"
)

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
	if err := at(t, root, nil).install(first); err != nil {
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
	if err := at(t, root, nil).upgrade(second); err != nil {
		t.Fatalf("upgrade 1.4.0: %v", err)
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
	if err := at(t, root, nil).install(dir); err != nil {
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
	err := at(t, root, nil).upgrade(dir)
	if !errors.Is(err, release.ErrNoRecord) {
		t.Errorf("got %v, want ErrNoRecord", err)
	}
}

func TestUpgradeReportsTheMachineNotReadyEvenWhenTheRecordCannotBeRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permissions cannot make the record unreadable")
	}
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	recordDir := filepath.Join(root, "var/lib/vessel/acme")
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(recordDir, "record.json")
	if err := os.WriteFile(recordPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(recordPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(recordPath, 0o644); err != nil {
			t.Errorf("chmod: %v", err)
		}
	})
	kinds := []driver.Driver{unreadyMachine{quadlet.New(root, (&podmanStub{}).run)}}
	err := at(t, root, kinds).upgrade(dir)
	if !errors.Is(err, driver.ErrNotReady) {
		t.Fatalf("got %v, want the machine-not-ready error", err)
	}
}

func TestUpgradeStopsTheServiceOfAUnitItDrops(t *testing.T) {
	root := t.TempDir()
	first := linkTestBundle(t, map[string]string{
		"vessel.yaml":     "name: acme\nversion: 1.3.0\n",
		"cache.container": "[Container]\nContainerName=cache\n",
	})
	if err := at(t, root, nil).install(first); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}

	second := linkTestBundle(t, map[string]string{"vessel.yaml": "name: acme\nversion: 1.4.0\n"})
	stub := &podmanStub{}
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).upgrade(second); err != nil {
		t.Fatalf("upgrade 1.4.0: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl stop cache.service") {
		t.Errorf("upgrade never stopped cache.service: %v", stub.calls)
	}
}

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
	if err := at(t, root, nil).install(first); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "etc/acme/a.txt")); err != nil {
		t.Fatalf("remove a.txt by hand: %v", err)
	}

	second := linkTestBundle(t, map[string]string{"vessel.yaml": "name: acme\nversion: 1.4.0\n"})
	var out bytes.Buffer
	job := at(t, root, nil).job(second, nil)
	if err := job.Upgrade(context.Background(), io.Discard, report.New(&out)); err != nil {
		t.Fatalf("upgrade 1.4.0: %v", err)
	}
	if strings.Contains(out.String(), "Removing") {
		t.Errorf("got %q, want no Removing line for a file already gone", out.String())
	}
}

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
	if err := at(t, root, nil).install(first); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}

	second := linkTestBundle(t, map[string]string{
		"vessel.yaml": "name: acme\nversion: 1.4.0\nactions:\n  - echo hi\n",
	})
	if err := at(t, root, nil).upgrade(second); err == nil {
		t.Fatal("upgrade succeeded despite a shell that refuses every action")
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/a.txt")); err != nil {
		t.Errorf("a.txt was removed before the failing action ran: %v", err)
	}
}

func TestUpgradeDisablesATimerItDrops(t *testing.T) {
	root := t.TempDir()
	first := linkTestBundle(t, map[string]string{
		"vessel.yaml":  "name: acme\nversion: 1.3.0\n",
		"backup.timer": "[Timer]\nOnCalendar=daily\n",
	})
	if err := at(t, root, nil).install(first); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}

	second := linkTestBundle(t, map[string]string{"vessel.yaml": "name: acme\nversion: 1.4.0\n"})
	stub := &podmanStub{}
	if err := at(t, root, []driver.Driver{quadlet.New(root, stub.run)}).upgrade(second); err != nil {
		t.Fatalf("upgrade 1.4.0: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl disable --now backup.timer") {
		t.Errorf("upgrade never disabled the timer it dropped: %v", stub.calls)
	}
}

func TestAFailedUpgradeKeepsTheStaleFileInTheRecord(t *testing.T) {
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
	if err := at(t, root, nil).install(first); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}

	dying := linkTestBundle(t, map[string]string{
		"vessel.yaml": "name: acme\nversion: 1.4.0\nactions:\n  - echo hi\n",
	})
	if err := at(t, root, nil).upgrade(dying); err == nil {
		t.Fatal("upgrade succeeded despite a shell that refuses every action")
	}

	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !slices.ContainsFunc(entry.Files, func(file release.Entry) bool { return file.Path == "etc/acme/a.txt" }) {
		t.Errorf("got %v, want a.txt still named, so the recovery can remove it", entry.Files)
	}
}

func TestUninstallAfterAFailedUpgradeRemovesTheStaleFile(t *testing.T) {
	root := t.TempDir()
	first := linkTestBundle(t, fixture(t, "stale-1.3"))
	if err := at(t, root, nil).install(first); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	if err := at(t, root, nil).upgrade(linkTestBundle(t, fixture(t, "stale-1.4"))); err == nil {
		t.Fatal("upgrade succeeded despite a shell that refuses every action")
	}
	if err := at(t, root, testKinds(root)).uninstall(io.Discard, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/a.txt")); !os.IsNotExist(err) {
		t.Errorf("a.txt is still on disk after the uninstall, err=%v", err)
	}
}

func TestUpgradeRefusesAMachineAnInstallLeftHalfway(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	if err := at(t, root, nil).upgrade(writeBundle(t, "acme", "1.5.0")); !errors.Is(err, ErrRecordOpen) {
		t.Fatalf("got %v, want ErrRecordOpen", err)
	}
}

func TestAnUpgradeKeepsAFileItDropsWhenEditedOnSite(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(linkTestBundle(t, fixture(t, "acme-1.3"))); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	writeFile(t, filepath.Join(root, "etc/acme/a.txt"), "edited on site\n")
	if err := at(t, root, nil).upgrade(linkTestBundle(t, fixture(t, "acme-1.4"))); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/a.txt")); err != nil {
		t.Errorf("the upgrade removed a file edited on this machine: %v", err)
	}
}

func TestUpgradeRefusesARecordAnOlderVesselWrote(t *testing.T) {
	root := t.TempDir()
	body, err := os.ReadFile("testdata/older-vessel/acme/record.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	writeFile(t, filepath.Join(root, "var/lib/vessel/acme/record.json"), string(body))
	err = at(t, root, nil).upgrade(writeBundle(t, "acme", "1.4.0"))
	if !errors.Is(err, ErrRecordTooOld) {
		t.Fatalf("got %v, want ErrRecordTooOld", err)
	}
	if errors.Is(err, ErrPartlyInstalled) {
		t.Error("a refusal before any change says the machine changed")
	}
}
