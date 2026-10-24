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

var ErrNoRecord = errors.New("this machine holds no record of that delivery")

var ErrRecordDoesNotMatch = errors.New("this machine no longer matches the record")

var ErrRecordNamesNoService = errors.New("an older vessel wrote this record and named no service, uninstall with the executable that installed it, then install")

func Status(ctx context.Context, out io.Writer, kinds []driver.Driver, root, name string) error {
	dir, err := release.Dir(root, name)
	if err != nil {
		return err
	}
	records := release.NewRecords(dir)
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
	changes := 0
	for _, entry := range record.Files {
		if reportFile(lines, root, entry) {
			changes++
		}
	}

	kind, err := driver.ByName(kinds, record.Machine)
	if err != nil {
		return err
	}
	if record.Format < release.Format {
		fmt.Fprintln(out, ErrRecordNamesNoService)
	}
	running, err := kind.Running(ctx, record.Services)
	if err != nil {
		return err
	}
	for _, service := range record.Services {
		state := "down"
		if slices.Contains(running, service) {
			state = "running"
		}
		lines.Line("Service", fmt.Sprintf("%s %s", service.Name, state))
	}
	if changes > 0 {
		return fmt.Errorf("%d of %d files: %w", changes, len(record.Files), ErrRecordDoesNotMatch)
	}
	return nil
}

func List(out io.Writer, root string) error {
	names, err := release.Deliveries(root)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintln(out, "this machine holds no delivery")
		return nil
	}
	for _, name := range names {
		dir, err := release.Dir(root, name)
		if err != nil {
			return err
		}
		record, _, err := release.NewRecords(dir).Read()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s %s\n", record.Name, record.Version)
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
	case disk.DigestOf(body) != entry.Digest:
		lines.Line("Differs", entry.Path)
	default:
		return false
	}
	return true
}
