package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/site"
)

func newPacked(self string) *cobra.Command {
	name := filepath.Base(os.Args[0])
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
	var flags answers
	var dry bool

	command := &cobra.Command{
		Use:   "install",
		Short: "Put the images and files on this machine",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withArtifact(self, func(artifact *bundle.Bundle) error {
				req, err := flags.request(artifact, command)
				if err != nil {
					return err
				}
				_, err = preview(machine(), dry).Run(command.Context(), req)
				return err
			})
		},
	}
	flags.bind(command)
	bindDryRunFlag(command, &dry)
	return command
}

func packedResume(self string) *cobra.Command {
	var flags answers
	var skip bool
	var dry bool

	command := &cobra.Command{
		Use:   "resume",
		Short: "Finish an install this executable started and never finished",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withArtifact(self, func(artifact *bundle.Bundle) error {
				req, err := flags.request(artifact, command)
				if err != nil {
					return err
				}
				_, err = resumer(preview(machine(), dry), skip).Run(command.Context(), req)
				return err
			})
		},
	}
	flags.bind(command)
	bindSkipFlag(command, &skip)
	bindDryRunFlag(command, &dry)
	return command
}

func packedUpgrade(self string) *cobra.Command {
	var flags answers
	var dry bool

	command := &cobra.Command{
		Use:   "upgrade",
		Short: "Install a newer version, refusing a machine that holds no record of this delivery",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withArtifact(self, func(artifact *bundle.Bundle) error {
				req, err := flags.request(artifact, command)
				if err != nil {
					return err
				}
				_, err = site.Upgrade(preview(machine(), dry)).Run(command.Context(), req)
				return err
			})
		},
	}
	flags.bind(command)
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
				status := site.Status{Root: machineRoot, Drivers: machines}
				_, err = status.Run(command.Context(), site.StatusRequest{Name: artifact.Config.Name, Out: command.OutOrStdout()})
				return err
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
				uninstall := site.Uninstall{Root: machineRoot, Drivers: machines}
				_, err = uninstall.Run(command.Context(), site.UninstallRequest{Name: artifact.Config.Name, Out: command.OutOrStdout()})
				return err
			})
		},
	}
	return command
}

func withArtifact(self string, run func(*bundle.Bundle) error) error {
	return withPayload(self, func(dir string) error {
		artifact, err := bundle.OpenNamed(dir)
		if err != nil {
			return err
		}
		return run(artifact)
	})
}

func withPayload(self string, run func(dir string) error) error {
	dir, err := os.MkdirTemp("", "vessel-bundle-*")
	if err != nil {
		return fmt.Errorf("unpack the carried bundle: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := site.Unpack(self, dir); err != nil {
		return err
	}
	return run(dir)
}
