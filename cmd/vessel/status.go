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
				_, err := site.List{Root: machineRoot}.Run(command.Context(), site.ListRequest{Out: command.OutOrStdout()})
				return err
			}
			status := site.Status{Root: machineRoot, Drivers: machines}
			_, err := status.Run(command.Context(), site.StatusRequest{Name: args[0], Out: command.OutOrStdout()})
			return err
		},
	}
	return command
}
