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

// jobOnDisk reads the bundle an operator named, checking it carries digests and a name first.
func jobOnDisk(dir string, values answers) (delivery.Install, error) {
	if !bundle.IsBundle(dir) {
		return delivery.Install{}, fmt.Errorf("%s %w", dir, bundle.ErrNotABundle)
	}
	artifact, err := bundle.OpenNamed(dir)
	if err != nil {
		return delivery.Install{}, err
	}
	return values.job(artifact)
}

// answers is what an operator gives the variables: plain values with --set, secrets with --set-file.
type answers struct {
	set   []string
	files []string
}

// bind gives command the repeated --set and --set-file flags.
func (a *answers) bind(command *cobra.Command) {
	command.Flags().StringArrayVar(&a.set, "set", nil, "answer a plain variable: --set NAME=value")
	command.Flags().StringArrayVar(&a.files, "set-file", nil, "answer a secret from a file only root reads: --set-file NAME=path")
}

// job wires an install of artifact to this machine, the machines this build knows and the system shell.
func (a answers) job(artifact *bundle.Bundle) (delivery.Install, error) {
	set, err := delivery.ParseSet(a.set)
	if err != nil {
		return delivery.Install{}, err
	}
	files, err := delivery.ReadSetFile(a.files)
	if err != nil {
		return delivery.Install{}, err
	}
	return delivery.Install{
		Artifact: artifact,
		Root:     machineRoot,
		Set:      set,
		SetFile:  files,
		Machines: machines,
		Shell:    machine.NewShell(machine.Sh),
	}, nil
}

// bindDryRunFlag gives command the --dry-run flag that prints the plan and changes nothing.
func bindDryRunFlag(command *cobra.Command, dry *bool) {
	command.Flags().BoolVar(dry, "dry-run", false, "check and print what would change, change nothing")
}

// preview turns job into its dry run when the flag asks for one.
func preview(job delivery.Install, dry bool) delivery.Install {
	if dry {
		return job.Preview()
	}
	return job
}
