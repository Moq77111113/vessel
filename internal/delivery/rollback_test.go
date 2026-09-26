package delivery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/report"
)

func TestThePreviousInstallerRollsBackAnUpgradeCutHalfway(t *testing.T) {
	root := t.TempDir()
	older := linkTestBundle(t, fixture(t, "acme-1.3"))
	if err := runInstall(t, root, older, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	if err := installWithABrokenImageLoad(t, root, linkTestBundle(t, fixture(t, "acme-1.4"))); err == nil {
		t.Fatal("upgrade succeeded with a broken image load")
	}
	if err := runInstall(t, root, older, nil); err != nil {
		t.Fatalf("roll back with the 1.3.0 installer: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/c.txt")); !os.IsNotExist(err) {
		t.Errorf("c.txt, carried only by 1.4.0, is still there: err=%v", err)
	}
	for path, want := range map[string]string{"etc/acme/a.txt": "a from 1.3\n", "etc/acme/b.txt": "b from 1.3\n"} {
		body, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(body) != want {
			t.Errorf("%s: got %q, want %q", path, body, want)
		}
	}
	entry, _, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if entry.Version != "1.3.0" || !entry.Done() {
		t.Errorf("got %s done=%v, want 1.3.0 done", entry.Version, entry.Done())
	}
}

func TestAThirdVersionStillRefusesAnUnfinishedUpgrade(t *testing.T) {
	root := t.TempDir()
	if err := runInstall(t, root, linkTestBundle(t, fixture(t, "acme-1.3")), nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	if err := installWithABrokenImageLoad(t, root, linkTestBundle(t, fixture(t, "acme-1.4"))); err == nil {
		t.Fatal("upgrade succeeded with a broken image load")
	}
	if err := runInstall(t, root, writeBundle(t, "acme", "1.5.0"), nil); !errors.Is(err, ErrRecordOpen) {
		t.Fatalf("got %v, want ErrRecordOpen", err)
	}
}

func TestAnUnfinishedUpgradeNamesTheInstallerToRollBackWith(t *testing.T) {
	root := t.TempDir()
	if err := runInstall(t, root, linkTestBundle(t, fixture(t, "acme-1.3")), nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	if err := installWithABrokenImageLoad(t, root, linkTestBundle(t, fixture(t, "acme-1.4"))); err == nil {
		t.Fatal("upgrade succeeded with a broken image load")
	}
	err := runInstall(t, root, writeBundle(t, "acme", "1.5.0"), nil)
	if err == nil || !strings.Contains(err.Error(), "the 1.3.0 installer") {
		t.Errorf("got %v, want the 1.3.0 installer named", err)
	}
}

func TestAFirstInstallCutHalfwayNamesNoPreviousVersion(t *testing.T) {
	root := t.TempDir()
	if err := installWithABrokenImageLoad(t, root, writeBundle(t, "acme", "1.4.0")); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	err := runInstall(t, root, writeBundle(t, "acme", "1.5.0"), nil)
	if !errors.Is(err, ErrRecordOpen) {
		t.Fatalf("got %v, want ErrRecordOpen", err)
	}
	if !strings.Contains(err.Error(), "resume, or uninstall") || strings.Contains(err.Error(), "installer") {
		t.Errorf("got %q, want resume or uninstall, and no previous installer", err.Error())
	}
}

func TestAnInstallCutAfterAnUninstallNamesNoRemovedVersion(t *testing.T) {
	root := t.TempDir()
	older := linkTestBundle(t, fixture(t, "acme-1.3"))
	if err := runInstall(t, root, older, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	if err := Uninstall(context.Background(), io.Discard, testKinds(), root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if err := installWithABrokenImageLoad(t, root, linkTestBundle(t, fixture(t, "acme-1.4"))); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	err := runInstall(t, root, older, nil)
	if !errors.Is(err, ErrRecordOpen) {
		t.Fatalf("got %v, want the removed 1.3.0 refused", err)
	}
	if strings.Contains(err.Error(), "1.3.0 installer") {
		t.Errorf("got %q, want no removed version offered", err.Error())
	}
}

func TestTheLastFinishedBuildOfTheSameVersionRollsBack(t *testing.T) {
	root := t.TempDir()
	first := writeBundle(t, "acme", "1.4.0")
	if err := runInstall(t, root, first, nil); err != nil {
		t.Fatalf("install build A: %v", err)
	}
	if err := installWithABrokenImageLoad(t, root, writeBundle(t, "acme", "1.4.0")); err == nil {
		t.Fatal("install of build B succeeded with a broken image load")
	}
	if err := runInstall(t, root, first, nil); err != nil {
		t.Fatalf("roll back with build A: %v", err)
	}
}

func TestARollbackOverAnOpenActionNamesIt(t *testing.T) {
	root := t.TempDir()
	older := linkTestBundle(t, fixture(t, "acme-1.3"))
	if err := runInstall(t, root, older, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	newer := linkTestBundle(t, fixture(t, "one-action"))
	openAnAction(t, root, newer)
	var out bytes.Buffer
	if err := jobFor(t, root, older, nil, nil).Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("roll back: %v", err)
	}
	if !strings.Contains(out.String(), "mkdir -p /srv/acme") {
		t.Errorf("got %q, want the open action named for the operator", out.String())
	}
}

func TestARollbackThroughUpgradeOverAnOpenActionNamesIt(t *testing.T) {
	root := t.TempDir()
	older := linkTestBundle(t, fixture(t, "acme-1.3"))
	if err := runInstall(t, root, older, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	openAnAction(t, root, linkTestBundle(t, fixture(t, "one-action")))
	var out bytes.Buffer
	if err := jobFor(t, root, older, nil, nil).Upgrade(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("roll back: %v", err)
	}
	if !strings.Contains(out.String(), "mkdir -p /srv/acme") {
		t.Errorf("got %q, want the open action named for the operator", out.String())
	}
}
