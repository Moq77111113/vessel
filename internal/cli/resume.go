package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/report"
)

func newResumeCommand() *cobra.Command {
	var set []string
	var skip bool
	var dry bool

	command := &cobra.Command{
		Use:   "resume <bundle>",
		Short: "Finish an install this bundle started and never finished",
		Long: "Skips every step and action the record says finished, then runs the rest.\n" +
			"Refuses when an action may have stopped halfway: check it, then --skip-action.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			job, err := jobOnDisk(args[0], set)
			if err != nil {
				return err
			}
			return resume(command.Context(), command.OutOrStdout(), report.New(command.ErrOrStderr()), preview(job, dry), skip)
		},
	}
	bindSetFlag(command, &set)
	bindSkipFlag(command, &skip)
	bindDryRunFlag(command, &dry)
	return command
}

// resume picks the one call the --skip-action flag names.
func resume(ctx context.Context, out io.Writer, work report.Report, job delivery.Install, skip bool) error {
	if skip {
		return job.SkipAction(ctx, out, work)
	}
	return job.Resume(ctx, out, work)
}

// bindSkipFlag gives command the --skip-action flag an operator answers an open action with.
func bindSkipFlag(command *cobra.Command, skip *bool) {
	command.Flags().BoolVar(skip, "skip-action", false,
		"treat the action the last install left halfway as finished, without running it")
}
