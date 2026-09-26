package delivery

import (
	"context"
	"slices"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/report"
)

// drops splits the files previous carried and next does not into those to remove and those edited on this machine, which stay.
func drops(tree *machine.Tree, previous, next []record.Entry) (remove, keep []string, err error) {
	for _, entry := range previous {
		if names(next, entry.Path) {
			continue
		}
		change, err := tree.Differs(entry.Path, entry.Digest)
		if err != nil {
			return nil, nil, err
		}
		if change {
			keep = append(keep, entry.Path)
			continue
		}
		remove = append(remove, entry.Path)
	}
	slices.Sort(remove)
	slices.Sort(keep)
	return remove, keep, nil
}

// removeGone stops the services of the files the new version drops, then removes them, keeping one edited on this machine.
func removeGone(ctx context.Context, work report.Report, host machine.Host, tree *machine.Tree,
	previous, next []record.Entry) error {
	remove, keep, err := drops(tree, previous, next)
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
func units(paths []string) []descriptor.File {
	files := make([]descriptor.File, 0, len(paths))
	for _, path := range paths {
		files = append(files, descriptor.File{Path: path})
	}
	return files
}
