package machine_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

// stub is a machine that recognizes one file name and does nothing else.
type stub struct {
	name string
	mark string
}

func (s stub) Name() string          { return s.name }
func (s stub) Detect(dir fs.FS) bool { _, err := fs.Stat(dir, s.mark); return err == nil }

func (s stub) Read(fs.FS) (descriptor.Manifest, error)         { return descriptor.Manifest{}, nil }
func (s stub) Owns(string) bool                                { return false }
func (s stub) Requires([]descriptor.File) []string             { return nil }
func (s stub) Check(context.Context, string) error             { return nil }
func (s stub) Secrets(context.Context) ([]string, error)       { return nil, nil }
func (s stub) AddSecret(context.Context, string, string) error { return nil }

func (s stub) AddImages(context.Context, report.Report, *machine.Layout) ([]string, error) {
	return nil, nil
}
func (s stub) Start(context.Context, []descriptor.File) error { return nil }

func (s stub) Services(context.Context, []descriptor.File) ([]machine.Service, error) {
	return nil, nil
}
func (s stub) Stop(context.Context, []descriptor.File) error { return nil }

func TestPickReturnsTheFirstMachineThatRecognizesTheSource(t *testing.T) {
	machines := []machine.Machine{
		stub{name: "compose", mark: "compose.yaml"},
		stub{name: "quadlet", mark: "web.container"},
	}
	dir := fstest.MapFS{"web.container": &fstest.MapFile{}}

	picked, err := machine.Pick(machines, dir)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if picked.Name() != "quadlet" {
		t.Errorf("got %q, want %q", picked.Name(), "quadlet")
	}
}

func TestPickKeepsTheOrderItWasGiven(t *testing.T) {
	machines := []machine.Machine{
		stub{name: "compose", mark: "web.container"},
		stub{name: "quadlet", mark: "web.container"},
	}

	picked, err := machine.Pick(machines, fstest.MapFS{"web.container": &fstest.MapFile{}})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if picked.Name() != "compose" {
		t.Errorf("got %q, want %q", picked.Name(), "compose")
	}
}

func TestPickWithoutAnyMachineFails(t *testing.T) {
	if _, err := machine.Pick(nil, fstest.MapFS{"web.container": &fstest.MapFile{}}); !errors.Is(err, machine.ErrNoMachine) {
		t.Fatalf("got %v, want ErrNoMachine", err)
	}
}

func TestPickNamesEveryKindItTriedWhenNothingMatches(t *testing.T) {
	machines := []machine.Machine{
		stub{name: "compose", mark: "compose.yaml"},
		stub{name: "quadlet", mark: "web.container"},
	}

	_, err := machine.Pick(machines, fstest.MapFS{"README.md": &fstest.MapFile{}})
	if !errors.Is(err, machine.ErrNoMachine) {
		t.Fatalf("got %v, want ErrNoMachine", err)
	}
	for _, name := range []string{"compose", "quadlet"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("got %q, want %q named in it", err.Error(), name)
		}
	}
}

func TestByNameFindsTheMachineThatBuiltABundle(t *testing.T) {
	machines := []machine.Machine{
		stub{name: "compose"},
		stub{name: "quadlet"},
	}
	found, err := machine.ByName(machines, "quadlet")
	if err != nil {
		t.Fatalf("by name: %v", err)
	}
	if found.Name() != "quadlet" {
		t.Errorf("got %q, want %q", found.Name(), "quadlet")
	}
}

func TestByNameRefusesABundleThisBuildCannotRead(t *testing.T) {
	_, err := machine.ByName([]machine.Machine{stub{name: "quadlet"}}, "compose")
	if !errors.Is(err, machine.ErrNoMachine) {
		t.Fatalf("got %v, want ErrNoMachine", err)
	}
	if !strings.Contains(err.Error(), "compose") {
		t.Errorf("got %q, want the kind it was asked for named in it", err.Error())
	}
}
