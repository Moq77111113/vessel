package site

import (
	"context"
	"errors"
	"fmt"

	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

var (
	ErrNothingToResume = errors.New("this machine holds no unfinished install of that delivery")
	ErrOtherBundle     = errors.New("this bundle is not the one the machine stopped installing")
	ErrActionUnknown   = errors.New("an action may have stopped halfway, check it by hand")
	ErrSetAfterValues  = errors.New("the unfinished install already stored its values, resume without --set")
	ErrRecordTooOld    = errors.New("an older vessel left this install unfinished, it cannot say how far it ran")
)

type Resume Install

func (r Resume) Run(ctx context.Context, req InstallRequest) (InstallResult, error) {
	install := Install(r)
	before, err := install.prepare(ctx, req.Artifact.Config)
	if err != nil {
		return InstallResult{}, err
	}
	if err := checkResume(req, before); err != nil {
		return InstallResult{}, err
	}
	if action, open := before.current.OpenAction(); open {
		return InstallResult{}, fmt.Errorf("%q: %w, then resume --skip-action", action.Command, ErrActionUnknown)
	}
	before.progress = before.current
	return install.run(ctx, req, before)
}

func checkResume(req InstallRequest, before state) error {
	if !before.exists || before.current.Done() {
		return fmt.Errorf("%s: %w", req.Artifact.Config.Name, ErrNothingToResume)
	}
	if before.current.Root != req.Artifact.Root {
		return fmt.Errorf("the machine stopped installing %s %s: %w",
			before.current.Name, before.current.Version, ErrOtherBundle)
	}
	if before.current.Format < release.Format {
		return fmt.Errorf("%s %s, run uninstall: %w", before.current.Name, before.current.Version, ErrRecordTooOld)
	}
	if name := plainSet(req.Artifact.Config.Variables, req.Set); name != "" && before.current.Has(release.StepValues) {
		return fmt.Errorf("--set %s: %w", name, ErrSetAfterValues)
	}
	return nil
}

func plainSet(variables []delivery.Variable, set map[string]string) string {
	for _, variable := range variables {
		if _, ok := set[variable.Name]; ok && !variable.Secret {
			return variable.Name
		}
	}
	return ""
}
