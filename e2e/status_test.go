package e2e

import (
	"strings"
	"testing"
)

func TestStatusNamesTheVersionAnInstallPutOnTheMachine(t *testing.T) {
	needPodman(t)
	takeSystemctl(t)

	root := t.TempDir()
	if err := runVessel("install", "--root", root, buildStack(t)); err != nil {
		t.Fatalf("install: %v", err)
	}
	out, err := runVesselCapture("status", "--root", root, "acme")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "acme 1.0") {
		t.Errorf("got %q, want the installed version named", out)
	}
}
