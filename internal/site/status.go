package site

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/driver"
)

var ErrRecordDoesNotMatch = errors.New("this machine no longer matches the record")

type Status struct {
	Root    string
	Drivers []driver.Driver
}

type StatusRequest struct {
	Name string
	Out  io.Writer
}

type StatusResult struct {
	Record  release.Record
	Changes int
}

func (s Status) Run(ctx context.Context, req StatusRequest) (StatusResult, error) {
	records, err := openRecords(s.Root, req.Name)
	if err != nil {
		return StatusResult{}, err
	}
	record, err := lastRecord(records, req.Name)
	if err != nil {
		return StatusResult{}, err
	}
	if err := header(req.Out, records, record); err != nil {
		return StatusResult{}, err
	}
	lines := report.New(req.Out)
	changes := s.files(lines, record)
	if err := s.services(ctx, req.Out, record); err != nil {
		return StatusResult{}, err
	}
	if changes > 0 {
		return StatusResult{Record: record, Changes: changes}, fmt.Errorf("%d of %d files: %w", changes, len(record.Files), ErrRecordDoesNotMatch)
	}
	return StatusResult{Record: record}, nil
}

func header(out io.Writer, records *release.Records, record release.Record) error {
	fmt.Fprintf(out, "%s %s, installed %s\n", record.Name, record.Version, record.Start.Format(time.RFC3339))
	if record.Insecure {
		fmt.Fprintln(out, bundle.InsecureWarning)
	}
	if !record.Done() {
		fmt.Fprintf(out, "this install never finished, it %s: %s\n", position(record), recovery(record.Prior))
	}
	if action, open := record.OpenAction(); open {
		fmt.Fprintf(out, "this action may have stopped halfway, check it by hand: %s\n", action.Command)
	}
	previous, err := records.Previous()
	if errors.Is(err, release.ErrNoRecord) {
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "previous %s, installed %s\n", previous.Version, previous.Start.Format(time.RFC3339))
	return nil
}

func (s Status) files(lines report.Report, record release.Record) int {
	changes := 0
	for _, entry := range record.Files {
		if reportFile(lines, s.Root, entry) {
			changes++
		}
	}
	return changes
}

func (s Status) services(ctx context.Context, out io.Writer, record release.Record) error {
	kind, err := byName(s.Drivers, record.Machine)
	if err != nil {
		return err
	}
	if record.Format < release.Format {
		fmt.Fprintln(out, ErrRecordTooOld)
	}
	running, err := kind.Running(ctx, record.Services)
	if err != nil {
		return err
	}
	lines := report.New(out)
	for _, service := range record.Services {
		state := "down"
		if slices.Contains(running, service) {
			state = "running"
		}
		lines.Line("Service", fmt.Sprintf("%s %s", service.Name, state))
	}
	return nil
}

func reportFile(lines report.Report, root string, entry release.Entry) bool {
	path := filepath.Join(root, entry.Path)
	body, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		lines.Line("Absent", entry.Path)
	case err != nil:
		lines.Line("Unreadable", entry.Path)
	case disk.Digest(body) != entry.Digest:
		lines.Line("Differs", entry.Path)
	default:
		return false
	}
	return true
}
