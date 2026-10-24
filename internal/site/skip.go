package site

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Moq77111113/vessel/internal/site/release"
)

var ErrNoOpenAction = errors.New("the unfinished install left no action halfway, run resume")

type SkipAction Install

func (s SkipAction) Run(ctx context.Context, req InstallRequest) (InstallResult, error) {
	install := Install(s)
	before, err := install.prepare(ctx, req.Artifact.Config)
	if err != nil {
		return InstallResult{}, err
	}
	if err := checkResume(req, before); err != nil {
		return InstallResult{}, err
	}
	if _, open := before.current.OpenAction(); !open {
		return InstallResult{}, fmt.Errorf("%s: %w", req.Artifact.Config.Name, ErrNoOpenAction)
	}
	before.progress = markLastActionDone(before.current)
	before.skip = before.progress.Actions[len(before.progress.Actions)-1].Command
	return install.run(ctx, req, before)
}

func markLastActionDone(current release.Record) release.Record {
	progress := current
	progress.Actions = slices.Clone(current.Actions)
	progress.Actions[len(progress.Actions)-1].End = time.Now().UTC()
	return progress
}
