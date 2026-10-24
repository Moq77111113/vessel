package site

import (
	"context"
	"fmt"

	"github.com/Moq77111113/vessel/internal/site/release"
)

type Upgrade Install

func (u Upgrade) Run(ctx context.Context, req InstallRequest) (InstallResult, error) {
	install := Install(u)
	before, err := install.prepare(ctx, req.Artifact.Config)
	if err != nil {
		return InstallResult{}, err
	}
	if !before.exists {
		return InstallResult{}, fmt.Errorf("%s: %w, run install instead", req.Artifact.Config.Name, release.ErrNoRecord)
	}
	if err := before.refuseOpen(req.Artifact.Root); err != nil {
		return InstallResult{}, err
	}
	if err := before.refuseOld(); err != nil {
		return InstallResult{}, err
	}
	before.bypass = bypass(before.current)
	return install.run(ctx, req, before)
}
