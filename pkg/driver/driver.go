// Package driver is the contract an install kind implements: read a delivery at build time, apply it on the machine.
package driver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

var ErrNoMachine = errors.New("no descriptor found")

var ErrUnknownMachine = errors.New("this build does not know that machine kind")

// Service is one unit of a delivery and whether the machine runs it.
type Service struct {
	Name    string
	Running bool
}

// Descriptor is the build side: it reads a delivery directory.
type Descriptor interface {
	Detect(fs.FS) bool
	Read(fs.FS) (delivery.Manifest, error)
	Owns(path string) bool
	Requires(files []delivery.File) []string
}

// Host is the machine side: it applies a delivery and reads back what is there.
type Host interface {
	Check(ctx context.Context, root string) error
	Secrets(ctx context.Context) ([]string, error)
	AddSecret(ctx context.Context, name, value string) error
	AddImages(ctx context.Context, work report.Report, layout *oci.Layout) ([]string, error)
	Start(ctx context.Context, files []delivery.File) error
	Services(ctx context.Context, files []delivery.File) ([]Service, error)
	Stop(ctx context.Context, files []delivery.File) error
}

// Machine is one deployment kind: quadlet on podman, compose on docker.
type Machine interface {
	Name() string
	Descriptor
	Host
}

// ByName returns the machine that built a bundle, so install can apply it.
func ByName(machines []Machine, name string) (Machine, error) {
	for _, candidate := range machines {
		if candidate.Name() == name {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownMachine, name)
}

// Pick returns the first machine that recognizes the directory, trying them in order.
func Pick(machines []Machine, dir fs.FS) (Machine, error) {
	for _, candidate := range machines {
		if candidate.Detect(dir) {
			return candidate, nil
		}
	}
	names := make([]string, len(machines))
	for i, candidate := range machines {
		names[i] = candidate.Name()
	}
	return nil, fmt.Errorf("%w, kinds tried: %s", ErrNoMachine, strings.Join(names, ", "))
}
