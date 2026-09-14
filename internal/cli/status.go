package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

// ErrNoRecord says this machine holds no record of that delivery.
var ErrNoRecord = errors.New("this machine holds no record of that delivery")

func newStatus() *cobra.Command {
	var root string
	command := &cobra.Command{
		Use:   "status [name]",
		Short: "Show what this machine holds, for one delivery or for every one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) == 0 {
				return list(command.OutOrStdout(), root)
			}
			return status(command.Context(), command.OutOrStdout(), machines, root, args[0])
		},
	}
	command.Flags().StringVar(&root, "root", "/", "read under this directory instead of /")
	command.Flags().MarkHidden("root")
	return command
}

// status prints the version this machine holds, whether the install finished, what changed
// on disk since, and what the machine runs now.
func status(ctx context.Context, out io.Writer, kinds []machine.Machine, root, name string) error {
	records, err := machine.NewRecords(root, name)
	if err != nil {
		return err
	}
	record, found, err := records.Read()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s: %w", name, ErrNoRecord)
	}
	fmt.Fprintf(out, "%s %s, installed %s\n", record.Name, record.Version, record.Start.Format(time.RFC3339))
	if !record.Done() {
		fmt.Fprintln(out, "this install never finished, run install again")
	}
	previous, held, err := records.Previous()
	if err != nil {
		return err
	}
	if held {
		fmt.Fprintf(out, "previous %s, installed %s\n", previous.Version, previous.Start.Format(time.RFC3339))
	}

	lines := report.New(out)
	files := make([]descriptor.File, 0, len(record.Files))
	for _, entry := range record.Files {
		files = append(files, descriptor.File{Path: entry.Path})
		reportDrift(lines, root, entry)
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
	return nil
}

// list names every delivery this machine holds a record of, with the version it holds.
func list(out io.Writer, root string) error {
	names, err := machine.Deliveries(root)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintln(out, "this machine holds no delivery")
		return nil
	}
	for _, name := range names {
		records, err := machine.NewRecords(root, name)
		if err != nil {
			return err
		}
		record, _, err := records.Read()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s %s\n", record.Name, record.Version)
	}
	return nil
}

// reportDrift names entry as Absent, Unreadable, or Differs, whichever matches disk now.
func reportDrift(lines report.Report, root string, entry machine.Entry) {
	path := filepath.Join(root, entry.Path)
	body, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		lines.Line("Absent", entry.Path)
	case err != nil:
		lines.Line("Unreadable", entry.Path)
	case machine.DigestOf(body) != entry.Digest:
		lines.Line("Differs", entry.Path)
	}
}
