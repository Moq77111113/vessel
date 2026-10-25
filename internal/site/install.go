package site

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/internal/site/values"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

type Install struct {
	Root    string
	Drivers []driver.Driver
	Shell   *disk.Shell
	preview bool
}

type InstallRequest struct {
	Artifact *bundle.Bundle
	Set      map[string]string
	SetFile  map[string]string
	Out      io.Writer
	Work     report.Report
}

type InstallResult struct {
	Record release.Record
}

func (i Install) Preview() Install {
	i.preview = true
	return i
}

func (i Install) Run(ctx context.Context, req InstallRequest) (InstallResult, error) {
	before, err := i.prepare(ctx, req.Artifact.Config)
	if err != nil {
		return InstallResult{}, err
	}
	if err := before.refuseOpen(req.Artifact.Root); err != nil {
		return InstallResult{}, err
	}
	if err := before.refuseOld(); err != nil {
		return InstallResult{}, err
	}
	before.bypass = bypass(before.current)
	return i.run(ctx, req, before)
}

func (i Install) run(ctx context.Context, req InstallRequest, before state) (InstallResult, error) {
	c, err := i.pass(ctx, req, before)
	if err != nil {
		return InstallResult{}, err
	}
	if i.preview {
		return InstallResult{}, i.dryRun(req, c)
	}
	if before.bypass != "" {
		fmt.Fprintf(req.Out, "rolling back over an action that may have stopped halfway, check it by hand: %s\n", before.bypass)
	}
	record, err := i.commit(ctx, req, c)
	if err != nil {
		return InstallResult{}, partway(err)
	}
	return InstallResult{Record: record}, nil
}

func (i Install) pass(ctx context.Context, req InstallRequest, before state) (*pass, error) {
	config := req.Artifact.Config
	secrets, err := before.host.Secrets(ctx)
	if err != nil {
		return nil, err
	}
	flags, err := values.Answers(config.Variables, req.Set, req.SetFile)
	if err != nil {
		return nil, err
	}
	store := values.NewStore(before.dir)
	resolution, err := values.Values{
		Store:   store,
		Shell:   i.shell(),
		Set:     flags,
		Secrets: values.Secrets{Names: secrets, Remove: before.host.RemoveHint},
	}.Resolve(ctx, config.Variables)
	if err != nil {
		return nil, err
	}
	files, err := delivery.Substitute(req.Artifact.Files, resolution.Values, delivery.SecretNames(config.Variables))
	if err != nil {
		return nil, err
	}
	if err := i.claim(config.Name, before.current, files); err != nil {
		return nil, err
	}
	return &pass{before: before, store: store, resolution: resolution, secrets: secrets, files: files}, nil
}

func (i Install) shell() *disk.Shell {
	if i.preview {
		return disk.NewShell(disk.Dry)
	}
	return i.Shell
}

func (i Install) commit(ctx context.Context, req InstallRequest, c *pass) (release.Record, error) {
	config := req.Artifact.Config
	next := release.Record{
		Name:     config.Name,
		Version:  config.Version,
		Machine:  config.Machine,
		Platform: config.Platform,
		Root:     req.Artifact.Root,
		Files:    release.Entries(c.files),
		Images:   imageDigests(config.Images),
		Services: config.Services,
		Secrets:  unionNames(c.before.current.Secrets, secretNames(c.resolution.Secrets)),
		Insecure: config.Insecure,
		Prior:    c.before.prior(),
		Start:    time.Now().UTC(),
	}
	steps, err := openSteps(c.before, next)
	if err != nil {
		return release.Record{}, err
	}
	c.steps = steps
	changes, err := i.apply(ctx, req, c)
	if err != nil {
		return release.Record{}, err
	}
	record, err := steps.close()
	if err != nil {
		return release.Record{}, err
	}
	finish(req, c, changes)
	return record, nil
}

func finish(req InstallRequest, c *pass, changes int) {
	config := req.Artifact.Config
	summary := fmt.Sprintf("%d of %d files changed", changes, len(c.files))
	if c.before.progress.Has(release.StepFiles) {
		summary = fmt.Sprintf("%d files already written", len(c.files))
	}
	report.New(req.Out).Line("Finished", fmt.Sprintf("%s %s installed and running: %d images, %s",
		config.Name, config.Version, len(config.Images), summary))
	if config.Insecure {
		fmt.Fprintln(req.Out, bundle.InsecureWarning)
	}
}
