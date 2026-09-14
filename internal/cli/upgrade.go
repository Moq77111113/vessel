package cli

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/report"
)

func newUpgradeCommand() *cobra.Command {
	var set []string

	command := &cobra.Command{
		Use:   "upgrade <bundle>",
		Short: "Install a newer version of a bundle on this machine",
		Long: "Does what install does, and refuses a machine that holds no record of this\n" +
			"delivery: run install there instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			job, err := jobOnDisk(args[0], set)
			if err != nil {
				return err
			}
			return job.Upgrade(command.Context(), command.OutOrStdout(), report.New(command.ErrOrStderr()))
		},
	}
	bindSetFlag(command, &set)
	return command
}
