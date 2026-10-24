package site

import (
	"context"
	"slices"

	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

func (c *pass) removeGone(ctx context.Context, work report.Report, tree *disk.Tree) error {
	current := c.before.current
	drop, err := release.Drops(tree, current.Files, c.steps.next.Files)
	if err != nil {
		return err
	}
	remove, keep := drop.Remove, drop.Keep
	for _, path := range keep {
		work.Line("Keeping", path+", edited on this machine")
	}
	if len(remove) == 0 {
		return nil
	}
	if err := c.before.host.Stop(ctx, servicesIn(current.Services, remove)); err != nil {
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

func servicesIn(services []delivery.Service, paths []string) []delivery.Service {
	var declared []delivery.Service
	for _, service := range services {
		if slices.Contains(paths, service.Path) {
			declared = append(declared, service)
		}
	}
	return declared
}
