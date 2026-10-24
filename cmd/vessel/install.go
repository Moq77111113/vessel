package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site"
	"github.com/Moq77111113/vessel/internal/site/disk"
)

func newInstallCommand() *cobra.Command {
	var values answers
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
			job, err := jobOnDisk(args[0], values)
			if err != nil {
				return err
			}
			return preview(job, dry).Run(command.Context(), command.OutOrStdout(), report.New(command.ErrOrStderr()))
		},
	}
	values.bind(command)
	bindDryRunFlag(command, &dry)
	return command
}

func jobOnDisk(dir string, values answers) (site.Install, error) {
	if !bundle.IsBundle(dir) {
		return site.Install{}, fmt.Errorf("%s %w", dir, bundle.ErrNotABundle)
	}
	artifact, err := bundle.OpenNamed(dir)
	if err != nil {
		return site.Install{}, err
	}
	return values.job(artifact)
}

type answers struct {
	set   []string
	files []string
}

func (a *answers) bind(command *cobra.Command) {
	command.Flags().StringArrayVar(&a.set, "set", nil, "answer a plain variable: --set NAME=value")
	command.Flags().StringArrayVar(&a.files, "set-file", nil, "answer a secret from a file only root reads: --set-file NAME=path")
}

func (a answers) job(artifact *bundle.Bundle) (site.Install, error) {
	set, err := site.ParseSet(a.set)
	if err != nil {
		return site.Install{}, err
	}
	files, err := site.ReadSetFile(a.files)
	if err != nil {
		return site.Install{}, err
	}
	return site.Install{
		Artifact: artifact,
		Root:     machineRoot,
		Set:      set,
		SetFile:  files,
		Machines: machines,
		Shell:    disk.NewShell(disk.Sh),
	}, nil
}

func bindDryRunFlag(command *cobra.Command, dry *bool) {
	command.Flags().BoolVar(dry, "dry-run", false, "check and print what would change, change nothing")
}

func preview(job site.Install, dry bool) site.Install {
	if dry {
		return job.Preview()
	}
	return job
}
