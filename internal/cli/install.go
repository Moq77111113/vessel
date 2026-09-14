package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

func newInstallCommand() *cobra.Command {
	var set []string

	command := &cobra.Command{
		Use:   "install <bundle>",
		Short: "Install a bundle on this machine",
		Long: "Checks the machine, verifies the bundle, puts its images into local storage and\n" +
			"its files on disk, removes any file a previous version put that this one does\n" +
			"not carry, then starts the services once.\n\n" +
			"For an operator, prefer an executable made by build: it needs no vessel binary.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			job, err := jobOnDisk(args[0], set)
			if err != nil {
				return err
			}
			return job.Run(command.Context(), command.OutOrStdout(), report.New(command.ErrOrStderr()))
		},
	}
	bindSetFlag(command, &set)
	return command
}

// jobOnDisk reads the bundle an operator named, verifying its signature before anything else.
func jobOnDisk(dir string, set []string) (delivery.Install, error) {
	if !bundle.IsBundle(dir) {
		return delivery.Install{}, fmt.Errorf("%s %w", dir, bundle.ErrNotABundle)
	}
	values, err := delivery.ParseSet(set)
	if err != nil {
		return delivery.Install{}, err
	}
	artifact, err := bundle.OpenVerified(dir)
	if err != nil {
		return delivery.Install{}, err
	}
	return newInstall(artifact, values), nil
}

// newInstall wires an install to this machine, the machines this build knows and the system shell.
func newInstall(artifact *bundle.Bundle, set map[string]string) delivery.Install {
	return delivery.Install{
		Artifact: artifact,
		Root:     machineRoot,
		Set:      set,
		Machines: machines,
		Shell:    machine.NewShell(machine.Sh),
	}
}

// bindSetFlag gives command the repeated --set flag an install answers its variables with.
func bindSetFlag(command *cobra.Command, set *[]string) {
	command.Flags().StringArrayVar(set, "set", nil, "answer a variable: --set NAME=value")
}
