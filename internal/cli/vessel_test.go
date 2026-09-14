package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/machine"
)

func TestTheCompositionRootNamesOneMachineKind(t *testing.T) {
	if len(machines) != 1 {
		t.Fatalf("got %d machines, want 1", len(machines))
	}
	if got, want := machines[0].Name(), "quadlet"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestVesselReadsQuadlet(t *testing.T) {
	kind, err := machine.Pick(machines, os.DirFS("../quadlet/testdata/stack"))
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got, want := kind.Name(), "quadlet"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestVesselOffersBuildInspectInstallStatusUninstallAndUpgrade(t *testing.T) {
	want := []string{"build", "inspect", "install", "status", "uninstall", "upgrade"}
	var got []string
	for _, command := range newVessel().Commands() {
		got = append(got, command.Name())
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestVesselRejectsAnUnknownCommand(t *testing.T) {
	err := runVessel(t, "deploy")
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "deploy") {
		t.Errorf("error does not name the command: %v", err)
	}
}

func TestBuildRefusesToRunWithoutAnOutputFile(t *testing.T) {
	err := runVessel(t, "build", t.TempDir())
	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "out") {
		t.Errorf("error does not name the missing flag: %v", err)
	}
}

func TestEveryCommandCarriesAShortDescription(t *testing.T) {
	for _, command := range New().Commands() {
		if command.Short == "" {
			t.Errorf("%s carries no short description", command.Name())
		}
	}
}

// runVessel runs the command line with its output captured.
func runVessel(t *testing.T, args ...string) error {
	t.Helper()
	vessel := New()
	vessel.SetArgs(args)
	vessel.SetOut(io.Discard)
	vessel.SetErr(io.Discard)
	return vessel.Execute()
}

func TestInstallOnADirectoryThatIsNotABundleSaysSo(t *testing.T) {
	err := runVessel(t, "install", t.TempDir(), "--root", t.TempDir())
	if !errors.Is(err, ErrNoBundle) {
		t.Errorf("got %v, want ErrNoBundle", err)
	}
}
