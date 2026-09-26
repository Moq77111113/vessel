package delivery

import (
	"context"
	"slices"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/report"
)

// gone names the files the previous version put and the new one does not carry.
func gone(previous, next []record.Entry) []string {
	paths := make(map[string]bool, len(next))
	for _, entry := range next {
		paths[entry.Path] = true
	}
	var missing []string
	for _, entry := range previous {
		if !paths[entry.Path] {
			missing = append(missing, entry.Path)
		}
	}
	slices.Sort(missing)
	return missing
}

// removeGone stops the services of a file the new version no longer carries, then removes it, keeping one edited on this machine.
func removeGone(ctx context.Context, work report.Report, host machine.Host, tree *machine.Tree,
	previous, next []record.Entry) error {
	var units []descriptor.File
	for _, entry := range previous {
		if names(next, entry.Path) {
			continue
		}
		change, err := tree.Differs(entry.Path, entry.Digest)
		if err != nil {
			return err
		}
		if change {
			work.Line("Keeping", entry.Path+", edited on this machine")
			continue
		}
		units = append(units, descriptor.File{Path: entry.Path})
	}
	if len(units) == 0 {
		return nil
	}
	if err := host.Stop(ctx, units); err != nil {
		return err
	}
	for _, unit := range units {
		ok, err := tree.Remove(unit.Path)
		if err != nil {
			return err
		}
		if ok {
			work.Line("Removing", unit.Path)
		}
	}
	return nil
}
