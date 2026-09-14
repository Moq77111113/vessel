package cli

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/delivery"
)

func newUninstall() *cobra.Command {
	command := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Take a delivery off this machine",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return delivery.Uninstall(command.Context(), command.OutOrStdout(), machines, machineRoot, args[0])
		},
	}
	return command
}
