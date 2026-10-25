package site

import (
	"context"

	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

// removeGone stops the services of the files the new version drops, then removes them, keeping one edited on this machine.
func removeGone(ctx context.Context, work report.Report, host driver.Host, tree *disk.Tree,
	previous, next []release.Entry) error {
	remove, keep, err := release.Drops(tree, previous, next)
	if err != nil {
		return err
	}
	for _, path := range keep {
		work.Line("Keeping", path+", edited on this machine")
	}
	if len(remove) == 0 {
		return nil
	}
	if err := host.Stop(ctx, units(remove)); err != nil {
		return err
	}
	for _, path := range remove {
		ok, err := tree.Remove(path)
		if err != nil {
			return err
		}
		if ok {
			work.Line("Removing", path)
		}
	}
	return nil
}

// units turns paths into the files a machine stops the services of.
func units(paths []string) []delivery.File {
	files := make([]delivery.File, 0, len(paths))
	for _, path := range paths {
		files = append(files, delivery.File{Path: path})
	}
	return files
}
