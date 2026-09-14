package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallRemovesTheUnitAndLeavesStatusReportingNoRecord(t *testing.T) {
	needPodman(t)
	takeSystemctl(t)

	root := t.TempDir()
	if err := runVessel("install", "--root", root, buildStack(t)); err != nil {
		t.Fatalf("install: %v", err)
	}
	unit := filepath.Join(root, "etc/containers/systemd/web.container")
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("the unit never reached the root: %v", err)
	}

	if err := runVessel("uninstall", "--root", root, "acme"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(unit); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("uninstall left the unit in place: %v", err)
	}
	_, err := runVesselCapture("status", "--root", root, "acme")
	if err == nil || !strings.Contains(err.Error(), "no record") {
		t.Errorf("got %v, want status to report no record after an uninstall", err)
	}
}
