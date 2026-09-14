package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

func newUninstall() *cobra.Command {
	var root string
	command := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Take a delivery off this machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return uninstall(command.Context(), command.OutOrStdout(), machines, root, args[0])
		},
	}
	bindRootFlag(command, &root, "remove under this directory instead of /")
	return command
}

// uninstall stops the services, removes exactly the files the record names, then retires the record.
func uninstall(ctx context.Context, out io.Writer, kinds []machine.Machine, root, name string) error {
	records := machine.NewRecords(root, name)
	record, found, err := records.Read()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s: %w", name, ErrNoRecord)
	}

	kind, err := machine.ByName(kinds, record.Machine)
	if err != nil {
		return err
	}

	files := make([]descriptor.File, 0, len(record.Files))
	for _, entry := range record.Files {
		files = append(files, descriptor.File{Path: entry.Path})
	}
	if err := kind.Stop(ctx, files); err != nil {
		return err
	}

	tree := machine.NewTree(root)
	removed := 0
	for _, entry := range record.Files {
		did, err := tree.Remove(entry.Path)
		if err != nil {
			return err
		}
		if did {
			removed++
		}
	}

	if err := records.Retire(); err != nil {
		return err
	}

	lines := report.New(out)
	lines.Line("Finished", fmt.Sprintf("%s %s removed: %d files", record.Name, record.Version, removed))
	reportWhatStays(out, root, name, record)
	return nil
}

// reportWhatStays names the secrets, the images, and the site values an uninstall leaves behind.
func reportWhatStays(out io.Writer, root, name string, record machine.Record) {
	if len(record.Secrets) > 0 {
		fmt.Fprintln(out, "\nThese secrets stay on this machine, another delivery may read them:")
		for _, secret := range record.Secrets {
			fmt.Fprintf(out, "  podman secret rm %s\n", secret)
		}
	}
	if len(record.Images) > 0 {
		fmt.Fprintln(out, "\nThese images stay in local storage, shared by digest with other deliveries:")
		for _, image := range record.Images {
			fmt.Fprintf(out, "  %s\n", image)
		}
	}
	fmt.Fprintf(out, "\nSite values stay at %s, a later install may read them.\n", machine.NewSite(root, name).Path())
}
