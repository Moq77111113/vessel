package site

import (
	"context"
	"errors"
	"fmt"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/driver"
)

var ErrUnknownDriver = errors.New("this build does not know that machine kind")

var ErrRecordOpen = errors.New("the last install on this machine never finished")

var ErrRecordNamesNoService = errors.New("an older vessel wrote this record and named no service, uninstall with the executable that installed it, then install")

type state struct {
	dir      string
	host     driver.Machine
	records  *release.Records
	current  release.Record
	exists   bool
	progress release.Record
	bypass   string
	skip     string
}

func (i Install) prepare(ctx context.Context, config bundle.Config) (state, error) {
	dir, err := release.Dir(i.Root, config.Name)
	if err != nil {
		return state{}, err
	}
	host, err := i.checkMachine(ctx, config.Machine)
	if err != nil {
		return state{}, err
	}
	records := release.NewRecords(dir)
	current, err := records.Read()
	exists := !errors.Is(err, release.ErrNoRecord)
	if exists && err != nil {
		return state{}, err
	}
	return state{dir: dir, host: host, records: records, current: current, exists: exists}, nil
}

func (i Install) checkMachine(ctx context.Context, name string) (driver.Machine, error) {
	host, err := byName(i.Drivers, name)
	if err != nil {
		return nil, err
	}
	if err := host.Check(ctx); err != nil {
		return nil, err
	}
	return host, nil
}

func (s state) refuseOpen(root string) error {
	if !s.exists || s.current.Done() {
		return nil
	}
	if s.current.Prior.Root == root {
		return nil
	}
	return fmt.Errorf("%s %s %s, %s: %w", s.current.Name, s.current.Version,
		position(s.current), recovery(s.current.Prior), ErrRecordOpen)
}

func (s state) refuseOld() error {
	if !s.exists {
		return nil
	}
	return refuseOldRecord(s.current)
}

func refuseOldRecord(current release.Record) error {
	if current.Format >= release.Format {
		return nil
	}
	return fmt.Errorf("%s %s: %w", current.Name, current.Version, ErrRecordNamesNoService)
}

func (s state) prior() release.Release {
	if s.exists && s.current.Done() {
		return release.Release{Version: s.current.Version, Root: s.current.Root}
	}
	return s.current.Prior
}

func bypass(current release.Record) string {
	if action, open := current.OpenAction(); open {
		return action.Command
	}
	return ""
}

func recovery(prior release.Release) string {
	if prior.Root == "" {
		return "run resume, or uninstall"
	}
	return fmt.Sprintf("run resume, or install with the %s installer", prior.Version)
}

func position(r release.Record) string {
	if len(r.Steps) == 0 {
		return "stopped before its first step"
	}
	return "stopped after " + string(r.Steps[len(r.Steps)-1])
}

func byName(drivers []driver.Driver, name string) (driver.Driver, error) {
	for _, candidate := range drivers {
		if candidate.Name() == name {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownDriver, name)
}
