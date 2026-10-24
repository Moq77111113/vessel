package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/values"
)

func newInstallCommand() *cobra.Command {
	var flags answers
	var dry bool

	command := &cobra.Command{
		Use:   "install <bundle>",
		Short: "Install a bundle on this machine",
		Long: "Checks the machine, opens the bundle, puts its images into local storage and\n" +
			"its files on disk, removes any file a previous version put that this one does\n" +
			"not carry, then starts the services once.\n\n" +
			"For an operator, prefer an executable made by build: it needs no vessel binary.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			req, err := requestOnDisk(args[0], flags, command)
			if err != nil {
				return err
			}
			_, err = preview(machine(), dry).Run(command.Context(), req)
			return err
		},
	}
	flags.bind(command)
	bindDryRunFlag(command, &dry)
	return command
}

func requestOnDisk(dir string, flags answers, command *cobra.Command) (site.InstallRequest, error) {
	if !bundle.IsBundle(dir) {
		return site.InstallRequest{}, fmt.Errorf("%s %w", dir, bundle.ErrNotABundle)
	}
	artifact, err := bundle.OpenNamed(dir)
	if err != nil {
		return site.InstallRequest{}, err
	}
	return flags.request(artifact, command)
}

type answers struct {
	set   []string
	files []string
}

func (a *answers) bind(command *cobra.Command) {
	command.Flags().StringArrayVar(&a.set, "set", nil, "answer a plain variable: --set NAME=value")
	command.Flags().StringArrayVar(&a.files, "set-file", nil, "answer a secret from a file only root reads: --set-file NAME=path")
}

func (a answers) request(artifact *bundle.Bundle, command *cobra.Command) (site.InstallRequest, error) {
	set, err := values.ParseSet(a.set)
	if err != nil {
		return site.InstallRequest{}, err
	}
	files, err := values.ReadSetFile(a.files)
	if err != nil {
		return site.InstallRequest{}, err
	}
	return site.InstallRequest{
		Artifact: artifact,
		Set:      set,
		SetFile:  files,
		Out:      command.OutOrStdout(),
		Work:     report.New(command.ErrOrStderr()),
	}, nil
}

func machine() site.Install {
	return site.Install{Root: machineRoot, Drivers: machines, Shell: disk.NewShell(disk.Sh)}
}

func bindDryRunFlag(command *cobra.Command, dry *bool) {
	command.Flags().BoolVar(dry, "dry-run", false, "check and print what would change, change nothing")
}

func preview(install site.Install, dry bool) site.Install {
	if dry {
		return install.Preview()
	}
	return install
}
