// Package driver is the contract an install kind implements: read a delivery at build time, apply it on the machine.
package driver

import (
	"context"
	"errors"
	"io/fs"

	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

// ErrNotReady says the machine lacks what the driver needs, before anything changes.
var ErrNotReady = errors.New("this machine is not ready")

// Reader reads a source directory at build time.
type Reader interface {
	Detect(fsys fs.FS) bool
	Read(fsys fs.FS) (delivery.Manifest, error)
}

// Machine applies a delivery on the site and reads back what runs.
type Machine interface {
	Check(ctx context.Context) error
	Secrets(ctx context.Context) ([]string, error)
	AddSecret(ctx context.Context, secret delivery.Secret) error
	RemoveHint(secret string) string
	AddImages(ctx context.Context, work report.Report, images *oci.Images) ([]string, error)
	Start(ctx context.Context, services []delivery.Service) error
	Stop(ctx context.Context, services []delivery.Service) error
	Running(ctx context.Context, services []delivery.Service) ([]delivery.Service, error)
}

// Driver is one install kind: what reads a delivery at build time and applies it on the machine.
type Driver interface {
	Reader
	Machine
	Name() string
}
