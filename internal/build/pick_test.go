package build

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

type kind struct {
	name string
	mark string
}

func (s kind) Name() string          { return s.name }
func (s kind) Detect(dir fs.FS) bool { _, err := fs.Stat(dir, s.mark); return err == nil }

func (s kind) Read(fs.FS) (delivery.Manifest, error)            { return delivery.Manifest{}, nil }
func (s kind) Check(context.Context) error                      { return nil }
func (s kind) Secrets(context.Context) ([]string, error)        { return nil, nil }
func (s kind) AddSecret(context.Context, delivery.Secret) error { return nil }
func (s kind) RemoveHint(string) string                         { return "" }
func (s kind) Start(context.Context, []delivery.Service) error  { return nil }
func (s kind) Stop(context.Context, []delivery.Service) error   { return nil }

func (s kind) AddImages(context.Context, report.Report, *oci.Images) ([]string, error) {
	return nil, nil
}

func (s kind) Running(context.Context, []delivery.Service) ([]delivery.Service, error) {
	return nil, nil
}

func TestPickReturnsTheFirstMachineThatRecognizesTheSource(t *testing.T) {
	machines := []driver.Driver{
		kind{name: "compose", mark: "compose.yaml"},
		kind{name: "quadlet", mark: "web.container"},
	}
	dir := fstest.MapFS{"web.container": &fstest.MapFile{}}

	kind, err := pick(machines, dir)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if kind.Name() != "quadlet" {
		t.Errorf("got %q, want %q", kind.Name(), "quadlet")
	}
}

func TestPickKeepsTheOrderItWasGiven(t *testing.T) {
	machines := []driver.Driver{
		kind{name: "compose", mark: "web.container"},
		kind{name: "quadlet", mark: "web.container"},
	}

	kind, err := pick(machines, fstest.MapFS{"web.container": &fstest.MapFile{}})
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if kind.Name() != "compose" {
		t.Errorf("got %q, want %q", kind.Name(), "compose")
	}
}

func TestPickWithoutAnyMachineFails(t *testing.T) {
	if _, err := pick(nil, fstest.MapFS{"web.container": &fstest.MapFile{}}); !errors.Is(err, ErrNoDriver) {
		t.Fatalf("got %v, want ErrNoMachine", err)
	}
}

func TestPickNamesEveryKindItTriedWhenNothingMatches(t *testing.T) {
	machines := []driver.Driver{
		kind{name: "compose", mark: "compose.yaml"},
		kind{name: "quadlet", mark: "web.container"},
	}

	_, err := pick(machines, fstest.MapFS{"README.md": &fstest.MapFile{}})
	if !errors.Is(err, ErrNoDriver) {
		t.Fatalf("got %v, want ErrNoMachine", err)
	}
	for _, name := range []string{"compose", "quadlet"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("got %q, want %q named in it", err.Error(), name)
		}
	}
}
