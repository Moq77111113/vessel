package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/site"
)

func newResumeCommand() *cobra.Command {
	var flags answers
	var skip bool
	var dry bool

	command := &cobra.Command{
		Use:   "resume <bundle>",
		Short: "Finish an install this bundle started and never finished",
		Long: "Skips every step and action the record says finished, then runs the rest.\n" +
			"Refuses when an action may have stopped halfway: check it, then --skip-action.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			req, err := requestOnDisk(args[0], flags, command)
			if err != nil {
				return err
			}
			_, err = resumer(preview(machine(), dry), skip).Run(command.Context(), req)
			return err
		},
	}
	flags.bind(command)
	bindSkipFlag(command, &skip)
	bindDryRunFlag(command, &dry)
	return command
}

type installer interface {
	Run(ctx context.Context, req site.InstallRequest) (site.InstallResult, error)
}

func resumer(install site.Install, skip bool) installer {
	if skip {
		return site.SkipAction(install)
	}
	return site.Resume(install)
}

func bindSkipFlag(command *cobra.Command, skip *bool) {
	command.Flags().BoolVar(skip, "skip-action", false,
		"treat the action the last install left halfway as finished, without running it")
}
