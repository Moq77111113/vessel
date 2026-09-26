package cli

import (
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
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

func TestVesselOffersSevenVerbs(t *testing.T) {
	if got, want := len(verbs()), 7; got != want {
		t.Errorf("got %d verbs, want %d: %v", got, want, verbs())
	}
}

func TestVesselListsItsVerbsInOrder(t *testing.T) {
	want := []string{"build", "inspect", "install", "resume", "status", "uninstall", "upgrade"}
	if got := verbs(); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// verbs names the commands vessel offers, in the order it lists them.
func verbs() []string {
	var names []string
	for _, command := range newVessel().Commands() {
		names = append(names, command.Name())
	}
	return names
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
	err := runVessel(t, "install", t.TempDir())
	if !errors.Is(err, bundle.ErrNotABundle) {
		t.Errorf("got %v, want ErrNoBundle", err)
	}
}

func TestAPackedInstallerOffersResume(t *testing.T) {
	var names []string
	for _, command := range newPacked("myapp").Commands() {
		names = append(names, command.Name())
	}
	if !slices.Contains(names, "resume") {
		t.Errorf("got %v, want resume among them", names)
	}
}

func TestResumeOffersSkipAction(t *testing.T) {
	if newResumeCommand().Flags().Lookup("skip-action") == nil {
		t.Error("resume offers no --skip-action")
	}
}
