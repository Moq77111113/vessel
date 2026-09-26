package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/report"
)

// newPacked is the command tree of an executable that carries its own bundle.
func newPacked(self string) *cobra.Command {
	name := filepath.Base(self)
	command := &cobra.Command{
		Use:           name,
		Short:         "Install " + name + " on this machine",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	command.AddCommand(packedInspect(self), packedInstall(self), packedResume(self), packedUpgrade(self),
		packedStatus(self), packedUninstall(self))
	return command
}

func packedInspect(self string) *cobra.Command {
	var into string

	command := &cobra.Command{
		Use:   "inspect",
		Short: "Show what is inside, without installing it",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withPayload(self, func(dir string) error {
				return inspectBundle(command.OutOrStdout(), dir, into)
			})
		},
	}
	bindEvidenceFlag(command, &into)
	return command
}

func packedInstall(self string) *cobra.Command {
	var set []string
	var dry bool

	command := &cobra.Command{
		Use:   "install",
		Short: "Put the images and files on this machine",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withJob(self, set, func(job delivery.Install) error {
				return preview(job, dry).Run(command.Context(), command.OutOrStdout(), report.New(command.ErrOrStderr()))
			})
		},
	}
	bindSetFlag(command, &set)
	bindDryRunFlag(command, &dry)
	return command
}

func packedResume(self string) *cobra.Command {
	var set []string
	var skip bool
	var dry bool

	command := &cobra.Command{
		Use:   "resume",
		Short: "Finish an install this executable started and never finished",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withJob(self, set, func(job delivery.Install) error {
				return resume(command.Context(), command.OutOrStdout(), report.New(command.ErrOrStderr()), preview(job, dry), skip)
			})
		},
	}
	bindSetFlag(command, &set)
	bindSkipFlag(command, &skip)
	bindDryRunFlag(command, &dry)
	return command
}

func packedUpgrade(self string) *cobra.Command {
	var set []string
	var dry bool

	command := &cobra.Command{
		Use:   "upgrade",
		Short: "Install a newer version, refusing a machine that holds no record of this delivery",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withJob(self, set, func(job delivery.Install) error {
				return preview(job, dry).Upgrade(command.Context(), command.OutOrStdout(), report.New(command.ErrOrStderr()))
			})
		},
	}
	bindSetFlag(command, &set)
	bindDryRunFlag(command, &dry)
	return command
}

func packedStatus(self string) *cobra.Command {
	command := &cobra.Command{
		Use:   "status",
		Short: "Show what this machine holds for this delivery",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withPayload(self, func(dir string) error {
				artifact, err := bundle.Open(dir)
				if err != nil {
					return err
				}
				return delivery.Status(command.Context(), command.OutOrStdout(), machines, machineRoot, artifact.Config.Name)
			})
		},
	}
	return command
}

func packedUninstall(self string) *cobra.Command {
	command := &cobra.Command{
		Use:   "uninstall",
		Short: "Take this delivery off this machine",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withPayload(self, func(dir string) error {
				artifact, err := bundle.Open(dir)
				if err != nil {
					return err
				}
				return delivery.Uninstall(command.Context(), command.OutOrStdout(), machines, machineRoot, artifact.Config.Name)
			})
		},
	}
	return command
}

// withJob lays the carried bundle down and hands the install it describes to run. A binary
// cannot vouch for the payload it carries, so the operator checks the file itself.
func withJob(self string, set []string, run func(delivery.Install) error) error {
	values, err := delivery.ParseSet(set)
	if err != nil {
		return err
	}
	return withPayload(self, func(dir string) error {
		artifact, err := bundle.OpenNamed(dir)
		if err != nil {
			return err
		}
		return run(newInstall(artifact, values))
	})
}

// withPayload lays the carried bundle down in a working directory and hands it to run.
func withPayload(self string, run func(dir string) error) error {
	dir, err := os.MkdirTemp("", "vessel-bundle-*")
	if err != nil {
		return fmt.Errorf("unpack the carried bundle: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := installer.Unpack(self, dir); err != nil {
		return err
	}
	return run(dir)
}
