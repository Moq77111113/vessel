package main

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/site"
)

func newUpgradeCommand() *cobra.Command {
	var flags answers
	var dry bool

	command := &cobra.Command{
		Use:   "upgrade <bundle>",
		Short: "Install a newer version of a bundle on this machine",
		Long: "Does what install does, and refuses a machine that holds no record of this\n" +
			"delivery: run install there instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			req, err := requestOnDisk(args[0], flags, command)
			if err != nil {
				return err
			}
			_, err = site.Upgrade(preview(machine(), dry)).Run(command.Context(), req)
			return err
		},
	}
	flags.bind(command)
	bindDryRunFlag(command, &dry)
	return command
}
