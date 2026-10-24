package site

import (
	"context"
	"fmt"
	"io"

	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/internal/site/values"
	"github.com/Moq77111113/vessel/pkg/driver"
)

// Uninstall stops the services, removes exactly the files the record names, then retires the record.
func Uninstall(ctx context.Context, out io.Writer, kinds []driver.Driver, root, name string) error {
	dir, err := release.Dir(root, name)
	if err != nil {
		return err
	}
	records := release.NewRecords(dir)
	store := values.NewStore(dir)
	record, found, err := records.Read()
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%s: %w", name, ErrNoRecord)
	}

	kind, err := driver.ByName(kinds, record.Machine)
	if err != nil {
		return err
	}

	if err := refuseOldRecord(record); err != nil {
		return err
	}
	if err := kind.Stop(ctx, record.Services); err != nil {
		return err
	}

	tree := disk.NewTree(root)
	count := 0
	var stays []string
	for _, entry := range record.Files {
		change, err := differs(tree, record, entry)
		if err != nil {
			return err
		}
		if change {
			stays = append(stays, entry.Path)
			continue
		}
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
	if len(stays) > 0 {
		fmt.Fprintln(out, "\nThese files stay, edited on this machine since the install:")
		for _, path := range stays {
			fmt.Fprintf(out, "  /%s\n", path)
		}
	}
	reportWhatStays(out, store.Path(), record, kind)
	return nil
}

// differs reports whether a finished install's file changed on this machine since.
// An open record holds the next version's digest for files it may not have written yet, so it proves no edit.
func differs(tree *disk.Tree, entry release.Record, file release.Entry) (bool, error) {
	if !entry.Done() {
		return false, nil
	}
	return tree.Differs(file.Path, file.Digest)
}

// reportWhatStays names the secrets, the images, and the site values an uninstall leaves behind.
func reportWhatStays(out io.Writer, values string, record release.Record, kind driver.Machine) {
	if len(record.Secrets) > 0 {
		fmt.Fprintln(out, "\nThese secrets stay on this machine, another delivery may read them:")
		for _, secret := range record.Secrets {
			fmt.Fprintf(out, "  %s\n", kind.RemoveHint(secret))
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
