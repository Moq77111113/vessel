package main

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/site"
)

func newUninstall() *cobra.Command {
	command := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Take a delivery off this machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			uninstall := site.Uninstall{Root: machineRoot, Drivers: machines}
			_, err := uninstall.Run(command.Context(), site.UninstallRequest{Name: args[0], Out: command.OutOrStdout()})
			return err
		},
	}
	return command
}
