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
		Use:   "status <name>",
		Short: "Show what this machine holds for a delivery",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
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
	record, found, err := machine.NewRecords(root, name).Read()
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

// reportDrift names entry as Missing, Unreadable, or Changed, whichever matches disk now.
func reportDrift(lines report.Report, root string, entry machine.Entry) {
	path := filepath.Join(root, entry.Path)
	body, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		lines.Line("Missing", entry.Path)
	case err != nil:
		lines.Line("Unreadable", entry.Path)
	case machine.DigestOf(body) != entry.Digest:
		lines.Line("Changed", entry.Path)
	}
}
