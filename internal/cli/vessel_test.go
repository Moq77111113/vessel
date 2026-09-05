package cli

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

func TestVesselReadsQuadlet(t *testing.T) {
	reader, err := descriptor.Pick(readers, os.DirFS("../quadlet/testdata/stack"))
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	if got, want := reader.Name(), "quadlet"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestVesselOffersTheThreeCommands(t *testing.T) {
	want := map[string]bool{"link": false, "load": false, "inspect": false}
	for _, command := range New().Commands() {
		want[command.Name()] = true
	}
	for command, offered := range want {
		if !offered {
			t.Errorf("vessel does not offer %q", command)
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

func TestLinkRefusesToRunWithoutAnOutputDirectory(t *testing.T) {
	err := runVessel(t, "link", t.TempDir())
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

func TestLoadOnADirectoryThatIsNotABundleSaysSo(t *testing.T) {
	err := runVessel(t, "load", t.TempDir(), "--root", t.TempDir())
	if !errors.Is(err, ErrNoBundle) {
		t.Errorf("got %v, want ErrNoBundle", err)
	}
}
