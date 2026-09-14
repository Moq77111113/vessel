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
	"time"

	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
)

func TestUninstallRemovesExactlyTheFilesTheRecordNames(t *testing.T) {
	root := t.TempDir()
	mine := filepath.Join(root, "etc/containers/systemd/web.container")
	theirs := filepath.Join(root, "etc/containers/systemd/other.container")
	writeFile(t, mine, "vessel put this")
	writeFile(t, theirs, "somebody else put this")
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []machine.Entry{{Path: "etc/containers/systemd/web.container"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := uninstall(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Error("uninstall kept a file it had put")
	}
	if _, err := os.Stat(theirs); err != nil {
		t.Error("uninstall removed a file it had never put")
	}
}

func TestUninstallRefusesARecordEntryThatLeavesTheRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "escape.txt")
	writeFile(t, outside, "not vessel's to touch")
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []machine.Entry{{Path: "../escape.txt"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	err := uninstall(context.Background(), io.Discard, testKinds(), root, "acme")
	if !errors.Is(err, machine.ErrPathEscapes) {
		t.Errorf("got %v, want ErrPathEscapes", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Error("uninstall removed a file outside the root")
	}
}

func TestUninstallCountsOnlyTheFilesItActuallyRemoved(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc/containers/systemd/web.container"), "vessel put this")
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []machine.Entry{
			{Path: "etc/containers/systemd/web.container"},
			{Path: "etc/containers/systemd/already-gone.container"},
		},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := uninstall(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out.String(), "removed: 1 files") {
		t.Errorf("got %q, want the count of files actually removed, not the count the record names", out.String())
	}
}

func TestUninstallStopsTheServiceBeforeRemovingItsFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc/containers/systemd/web.container")
	writeFile(t, path, "vessel put this")
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []machine.Entry{{Path: "etc/containers/systemd/web.container"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	kinds := []machine.Machine{brokenStop{quadlet.New((&podmanStub{}).run)}}
	err := uninstall(context.Background(), io.Discard, kinds, root, "acme")
	if !errors.Is(err, errBrokenStop) {
		t.Fatalf("got %v, want errBrokenStop", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("uninstall removed a file before stopping the service failed")
	}
}

func TestUninstallNamesTheSecretsItLeavesOnTheMachine(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet", Secrets: []string{"DB_PASSWORD"},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := uninstall(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out.String(), "DB_PASSWORD") {
		t.Errorf("got %q, want the secret named so the operator decides", out.String())
	}
}

func TestUninstallNamesTheImagesItLeavesInLocalStorage(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet", Images: []string{"sha256:aaaa"},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := uninstall(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out.String(), "sha256:aaaa") {
		t.Errorf("got %q, want the image named, another delivery may share it", out.String())
	}
}

func TestUninstallNamesTheSiteValuesItLeaves(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := uninstall(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out.String(), siteFor(t, root, "acme").Path()) {
		t.Errorf("got %q, want the site values path named", out.String())
	}
}

func TestUninstallLeavesStatusReportingNoRecord(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	if err := uninstall(context.Background(), io.Discard, testKinds(), root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	err := status(context.Background(), io.Discard, testKinds(), root, "acme")
	if !errors.Is(err, ErrNoRecord) {
		t.Errorf("got %v, want ErrNoRecord", err)
	}
}

func TestUninstallFailsOnAMachineThatHoldsNoRecord(t *testing.T) {
	err := uninstall(context.Background(), io.Discard, testKinds(), t.TempDir(), "acme")
	if !errors.Is(err, ErrNoRecord) {
		t.Errorf("got %v, want ErrNoRecord", err)
	}
}

// brokenStop wraps a machine.Machine and fails Stop, so a test drives an uninstall past the
// point where it must not have touched a file yet.
type brokenStop struct {
	machine.Machine
}

// errBrokenStop is the failure brokenStop.Stop reports.
var errBrokenStop = errors.New("broken stop")

func (brokenStop) Stop(context.Context, []descriptor.File) error { return errBrokenStop }

// writeFile creates path with body, making its parent directories as needed.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestUninstallRefusesANameThatLeavesTheTargetRoot(t *testing.T) {
	err := uninstall(context.Background(), io.Discard, testKinds(), t.TempDir(), "../../../etc/cron.daily")
	if !errors.Is(err, delivery.ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func TestUninstallDisablesATimerTheDeliveryCarried(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc/systemd/system/backup.timer"), "[Timer]\n")
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []machine.Entry{{Path: "etc/systemd/system/backup.timer"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	stub := &podmanStub{}
	if err := uninstall(context.Background(), io.Discard, []machine.Machine{quadlet.New(stub.run)}, root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !slices.Contains(stub.calls, "systemctl disable --now backup.timer") {
		t.Errorf("uninstall never disabled the timer: %v", stub.calls)
	}
}
