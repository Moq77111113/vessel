package delivery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/report"
)

var ErrNoRecord = errors.New("this machine holds no record of that delivery")

var ErrRecordDoesNotMatch = errors.New("this machine no longer matches the record")

// Status prints what this machine holds for a delivery, and fails when disk no longer matches it.
func Status(ctx context.Context, out io.Writer, kinds []machine.Machine, root, name string) error {
	dir, err := dirFor(root, name)
	if err != nil {
		return err
	}
	records := record.NewRecords(dir)
	record, found, err := records.Read()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s: %w", name, ErrNoRecord)
	}
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
	previous, found, err := records.Previous()
	if err != nil {
		return err
	}
	if found {
		fmt.Fprintf(out, "previous %s, installed %s\n", previous.Version, previous.Start.Format(time.RFC3339))
	}

	lines := report.New(out)
	files := make([]descriptor.File, 0, len(record.Files))
	changes := 0
	for _, entry := range record.Files {
		files = append(files, descriptor.File{Path: entry.Path})
		if reportFile(lines, root, entry) {
			changes++
		}
	}

	kind, err := machine.ByName(kinds, record.Machine)
	if err != nil {
		return err
	}
	services, err := kind.Services(ctx, files)
	if err != nil {
		return err
	}
	for _, service := range services {
		state := "down"
		if service.Running {
			state = "running"
		}
		lines.Line("Service", fmt.Sprintf("%s %s", service.Name, state))
	}
	if changes > 0 {
		return fmt.Errorf("%d of %d files: %w", changes, len(record.Files), ErrRecordDoesNotMatch)
	}
	return nil
}

// List names every delivery this machine holds a record of, with the version it holds.
func List(out io.Writer, root string) error {
	names, err := record.Deliveries(deliveriesDir(root))
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintln(out, "this machine holds no delivery")
		return nil
	}
	for _, name := range names {
		dir, err := dirFor(root, name)
		if err != nil {
			return err
		}
		record, _, err := record.NewRecords(dir).Read()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s %s\n", record.Name, record.Version)
	}
	return nil
}

// reportFile names entry as Absent, Unreadable or Differs, and reports whether disk still matches it.
func reportFile(lines report.Report, root string, entry record.Entry) bool {
	path := filepath.Join(root, entry.Path)
	body, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		lines.Line("Absent", entry.Path)
	case err != nil:
		lines.Line("Unreadable", entry.Path)
	case machine.DigestOf(body) != entry.Digest:
		lines.Line("Differs", entry.Path)
	default:
		return false
	}
	return true
}
