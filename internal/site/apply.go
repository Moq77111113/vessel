package site

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/internal/site/values"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

var (
	ErrPartlyInstalled = errors.New("this machine is partway through the install")
	ErrServiceIsDown   = errors.New("did not start")
)

type pass struct {
	before     state
	store      *values.Store
	resolution values.Resolution
	secrets    []string
	files      []delivery.File
	steps      *steps
}

func (i Install) apply(ctx context.Context, req InstallRequest, c *pass) (int, error) {
	config := req.Artifact.Config
	host := c.before.host
	tree := disk.NewTree(i.Root)
	var changes int
	if err := c.steps.run(release.StepValues, func() error {
		return c.store.Write(c.resolution.Values)
	}); err != nil {
		return 0, err
	}
	if err := c.steps.run(release.StepActions, func() error {
		return runActions(ctx, quiet(config.Actions, i.Shell), c.steps)
	}); err != nil {
		return 0, err
	}
	if err := c.steps.run(release.StepFiles, func() error {
		var err error
		changes, err = c.writeFiles(ctx, req, tree)
		return err
	}); err != nil {
		return 0, err
	}
	if err := c.steps.run(release.StepSecrets, func() error {
		return addSecrets(ctx, host, c.resolution.Secrets)
	}); err != nil {
		return 0, err
	}
	if err := c.steps.run(release.StepImages, func() error {
		return addImages(ctx, req, host)
	}); err != nil {
		return 0, err
	}
	if err := c.steps.run(release.StepServices, func() error {
		reportMissingSecrets(req.Out, c.secrets, config.Secrets)
		return startServices(ctx, host, config.Services)
	}); err != nil {
		return 0, err
	}
	if err := c.steps.run(release.StepAfter, func() error {
		return runActions(ctx, after(config.After, i.Shell, req.Out), c.steps)
	}); err != nil {
		return 0, err
	}
	return changes, nil
}

type action struct {
	command string
	run     func(ctx context.Context) error
}

type actions struct {
	step release.Step
	list []action
}

func quiet(commands []string, shell *disk.Shell) actions {
	todo := actions{step: release.StepActions}
	for _, command := range commands {
		todo.list = append(todo.list, action{command: command, run: func(ctx context.Context) error {
			return shell.Stream(ctx, command, io.Discard)
		}})
	}
	return todo
}

func after(entries []delivery.AfterAction, shell *disk.Shell, out io.Writer) actions {
	todo := actions{step: release.StepAfter}
	for _, entry := range entries {
		todo.list = append(todo.list, action{command: entry.Command, run: afterRun(entry, shell, out)})
	}
	return todo
}

func afterRun(entry delivery.AfterAction, shell *disk.Shell, out io.Writer) func(ctx context.Context) error {
	if entry.Mute {
		return func(ctx context.Context) error { return shell.Mute(ctx, entry.Command) }
	}
	return func(ctx context.Context) error { return shell.Stream(ctx, entry.Command, out) }
}

func runActions(ctx context.Context, todo actions, s *steps) error {
	for _, next := range todo.list[min(count(s.opening.Actions, todo.step), len(todo.list)):] {
		if err := s.action(todo.step, next.command, func() error { return next.run(ctx) }); err != nil {
			return err
		}
	}
	return nil
}

func (c *pass) writeFiles(ctx context.Context, req InstallRequest, tree *disk.Tree) (int, error) {
	changes := 0
	for _, file := range c.files {
		wrote, err := tree.Write(file)
		if err != nil {
			return changes, err
		}
		if wrote {
			changes++
		}
	}
	if !c.before.exists {
		return changes, nil
	}
	return changes, c.removeGone(ctx, req.Work, tree)
}

func addSecrets(ctx context.Context, host driver.Machine, secrets map[string]string) error {
	for name, value := range secrets {
		if err := host.AddSecret(ctx, delivery.Secret{Name: name, Value: value}); err != nil {
			return err
		}
	}
	return nil
}

func addImages(ctx context.Context, req InstallRequest, host driver.Machine) error {
	images, err := oci.Open(req.Artifact.LayoutDir, req.Artifact.Root)
	if err != nil {
		return err
	}
	_, err = host.AddImages(ctx, req.Work, images)
	return err
}

func startServices(ctx context.Context, host driver.Machine, services []delivery.Service) error {
	if err := host.Start(ctx, services); err != nil {
		return err
	}
	running, err := host.Running(ctx, services)
	if err != nil {
		return err
	}
	var down []string
	for _, service := range services {
		if !slices.Contains(running, service) {
			down = append(down, service.Name)
		}
	}
	if len(down) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %w", strings.Join(down, ", "), ErrServiceIsDown)
}

func reportMissingSecrets(out io.Writer, secrets, read []string) {
	var missing []string
	for _, name := range read {
		if !slices.Contains(secrets, name) {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return
	}
	fmt.Fprintf(out, "\nThese secrets are not on this machine yet, the services read them:\n")
	for _, name := range missing {
		fmt.Fprintf(out, "  %s\n", name)
	}
}

func partway(err error) error {
	if errors.Is(err, ErrPartlyInstalled) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrPartlyInstalled, err)
}
