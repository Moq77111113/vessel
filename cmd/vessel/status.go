package main

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/site"
)

func newStatus() *cobra.Command {
	command := &cobra.Command{
		Use:   "status [name]",
		Short: "Show what this machine holds, for one delivery or for every one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if len(args) == 0 {
				return site.List(command.OutOrStdout(), machineRoot)
			}
			return site.Status(command.Context(), command.OutOrStdout(), machines, machineRoot, args[0])
		},
	}
	return command
}
