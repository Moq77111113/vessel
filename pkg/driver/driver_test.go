package driver_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

type stub struct {
	name string
	mark string
}

func (s stub) Name() string          { return s.name }
func (s stub) Detect(dir fs.FS) bool { _, err := fs.Stat(dir, s.mark); return err == nil }

func (s stub) Read(fs.FS) (delivery.Manifest, error)            { return delivery.Manifest{}, nil }
func (s stub) Check(context.Context) error                      { return nil }
func (s stub) Secrets(context.Context) ([]string, error)        { return nil, nil }
func (s stub) AddSecret(context.Context, delivery.Secret) error { return nil }
func (s stub) RemoveHint(string) string                         { return "" }
func (s stub) Start(context.Context, []delivery.Service) error  { return nil }
func (s stub) Stop(context.Context, []delivery.Service) error   { return nil }

func (s stub) AddImages(context.Context, report.Report, *oci.Images) ([]string, error) {
	return nil, nil
}

func (s stub) Running(context.Context, []delivery.Service) ([]delivery.Service, error) {
	return nil, nil
}

func TestPickReturnsTheFirstMachineThatRecognizesTheSource(t *testing.T) {
	machines := []driver.Driver{
		stub{name: "compose", mark: "compose.yaml"},
		stub{name: "quadlet", mark: "web.container"},
	}
	dir := fstest.MapFS{"web.container": &fstest.MapFile{}}

	kind, err := driver.Pick(machines, dir)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if kind.Name() != "quadlet" {
		t.Errorf("got %q, want %q", kind.Name(), "quadlet")
	}
}

func TestPickKeepsTheOrderItWasGiven(t *testing.T) {
	machines := []driver.Driver{
		stub{name: "compose", mark: "web.container"},
		stub{name: "quadlet", mark: "web.container"},
	}

	kind, err := driver.Pick(machines, fstest.MapFS{"web.container": &fstest.MapFile{}})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if kind.Name() != "compose" {
		t.Errorf("got %q, want %q", kind.Name(), "compose")
	}
}

func TestPickWithoutAnyMachineFails(t *testing.T) {
	if _, err := driver.Pick(nil, fstest.MapFS{"web.container": &fstest.MapFile{}}); !errors.Is(err, driver.ErrNoDriver) {
		t.Fatalf("got %v, want ErrNoMachine", err)
	}
}

func TestPickNamesEveryKindItTriedWhenNothingMatches(t *testing.T) {
	machines := []driver.Driver{
		stub{name: "compose", mark: "compose.yaml"},
		stub{name: "quadlet", mark: "web.container"},
	}

	_, err := driver.Pick(machines, fstest.MapFS{"README.md": &fstest.MapFile{}})
	if !errors.Is(err, driver.ErrNoDriver) {
		t.Fatalf("got %v, want ErrNoMachine", err)
	}
	for _, name := range []string{"compose", "quadlet"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("got %q, want %q named in it", err.Error(), name)
		}
	}
}

func TestByNameFindsTheMachineThatBuiltABundle(t *testing.T) {
	machines := []driver.Driver{
		stub{name: "compose"},
		stub{name: "quadlet"},
	}
	found, err := driver.ByName(machines, "quadlet")
	if err != nil {
		t.Fatalf("by name: %v", err)
	}
	if found.Name() != "quadlet" {
		t.Errorf("got %q, want %q", found.Name(), "quadlet")
	}
}

func TestByNameRefusesABundleThisBuildCannotRead(t *testing.T) {
	_, err := driver.ByName([]driver.Driver{stub{name: "quadlet"}}, "compose")
	if !errors.Is(err, driver.ErrUnknownDriver) {
		t.Fatalf("got %v, want ErrUnknownMachine", err)
	}
	if !strings.Contains(err.Error(), "compose") {
		t.Errorf("got %q, want the kind it was asked for named in it", err.Error())
	}
}
