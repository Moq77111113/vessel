package delivery

import (
	"context"
	"fmt"
	"io"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site"
)

// Uninstall stops the services, removes exactly the files the record names, then retires the record.
func Uninstall(ctx context.Context, out io.Writer, kinds []machine.Machine, root, name string) error {
	dir, err := dirFor(root, name)
	if err != nil {
		return err
	}
	records := record.NewRecords(dir)
	site := site.NewStore(dir)
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
	count := 0
	for _, entry := range record.Files {
		ok, err := tree.Remove(entry.Path)
		if err != nil {
			return err
		}
		if ok {
			count++
		}
	}

	if err := records.Remove(); err != nil {
		return err
	}

	lines := report.New(out)
	lines.Line("Finished", fmt.Sprintf("%s %s removed: %d files", record.Name, record.Version, count))
	reportWhatStays(out, site.Path(), record)
	return nil
}

// reportWhatStays names the secrets, the images, and the site values an uninstall leaves behind.
func reportWhatStays(out io.Writer, values string, record record.Record) {
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
	fmt.Fprintf(out, "\nSite values stay at %s, a later install may read them.\n", values)
}
