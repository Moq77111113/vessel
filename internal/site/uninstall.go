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

type Uninstall struct {
	Root    string
	Drivers []driver.Driver
}

type UninstallRequest struct {
	Name string
	Out  io.Writer
}

type UninstallResult struct {
	Removed int
	Kept    []string
}

func (u Uninstall) Run(ctx context.Context, req UninstallRequest) (UninstallResult, error) {
	dir, err := release.Dir(u.Root, req.Name)
	if err != nil {
		return UninstallResult{}, err
	}
	records := release.NewRecords(dir)
	record, err := lastRecord(records, req.Name)
	if err != nil {
		return UninstallResult{}, err
	}
	kind, err := byName(u.Drivers, record.Machine)
	if err != nil {
		return UninstallResult{}, err
	}
	if err := refuseOldRecord(record); err != nil {
		return UninstallResult{}, err
	}
	if err := kind.Stop(ctx, record.Services); err != nil {
		return UninstallResult{}, err
	}
	result, err := u.removeFiles(record)
	if err != nil {
		return UninstallResult{}, err
	}
	if err := records.Remove(); err != nil {
		return UninstallResult{}, err
	}
	reportRemoved(req.Out, record, result)
	reportWhatStays(req.Out, record, kind)
	fmt.Fprintf(req.Out, "\nSite values stay at %s, a later install may read them.\n", values.NewStore(dir).Path())
	return result, nil
}

func (u Uninstall) removeFiles(record release.Record) (UninstallResult, error) {
	tree := disk.NewTree(u.Root)
	var result UninstallResult
	for _, entry := range record.Files {
		change, err := differs(tree, record, entry)
		if err != nil {
			return UninstallResult{}, err
		}
		if change {
			result.Kept = append(result.Kept, entry.Path)
			continue
		}
		ok, err := tree.Remove(entry.Path)
		if err != nil {
			return UninstallResult{}, err
		}
		if ok {
			result.Removed++
		}
	}
	return result, nil
}

func differs(tree *disk.Tree, entry release.Record, file release.Entry) (bool, error) {
	if !entry.Done() {
		return false, nil
	}
	return tree.Differs(file.Path, file.Digest)
}

func reportRemoved(out io.Writer, record release.Record, result UninstallResult) {
	report.New(out).Line("Finished", fmt.Sprintf("%s %s removed: %d files", record.Name, record.Version, result.Removed))
	if len(result.Kept) > 0 {
		fmt.Fprintln(out, "\nThese files stay, edited on this machine since the install:")
		for _, path := range result.Kept {
			fmt.Fprintf(out, "  /%s\n", path)
		}
	}
}

func reportWhatStays(out io.Writer, record release.Record, kind driver.Machine) {
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
}
