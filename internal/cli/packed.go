package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

// newPacked is the command tree of an executable that carries its own bundle.
// It has one job, so it offers no way to build anything.
func newPacked(self string) *cobra.Command {
	var root string
	var set []string
	var upgradeRoot string
	var upgradeSet []string
	var statusRoot string
	var uninstallRoot string

	name := filepath.Base(self)
	packed := &cobra.Command{
		Use:           name,
		Short:         "Install " + name + " on this machine",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	inspect := &cobra.Command{
		Use:   "inspect",
		Short: "Show what is inside, without installing it",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withPayload(self, func(dir string) error {
				return inspectBundle(command.OutOrStdout(), dir)
			})
		},
	}

	install := &cobra.Command{
		Use:   "install",
		Short: "Put the images and files on this machine",
		Args:  cobra.NoArgs,
		RunE:  runLoad(self, &root, &set, modeInstall),
	}
	bindRootFlag(install, &root, "install under this directory instead of /")
	install.Flags().StringArrayVar(&set, "set", nil, "answer a variable: --set NAME=value")

	upgrade := &cobra.Command{
		Use:   "upgrade",
		Short: "Install a newer version, refusing a machine that holds no record of this delivery",
		Args:  cobra.NoArgs,
		RunE:  runLoad(self, &upgradeRoot, &upgradeSet, modeUpgrade),
	}
	bindRootFlag(upgrade, &upgradeRoot, "install under this directory instead of /")
	upgrade.Flags().StringArrayVar(&upgradeSet, "set", nil, "answer a variable: --set NAME=value")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show what this machine holds for this delivery",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withPayload(self, func(dir string) error {
				artifact, err := bundle.Open(dir)
				if err != nil {
					return err
				}
				return status(command.Context(), command.OutOrStdout(), machines, statusRoot, artifact.Config.Name)
			})
		},
	}
	bindRootFlag(statusCmd, &statusRoot, "read under this directory instead of /")

	uninstallCmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Take this delivery off this machine",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return withPayload(self, func(dir string) error {
				artifact, err := bundle.Open(dir)
				if err != nil {
					return err
				}
				return uninstall(command.Context(), command.OutOrStdout(), machines, uninstallRoot, artifact.Config.Name)
			})
		},
	}
	bindRootFlag(uninstallCmd, &uninstallRoot, "remove under this directory instead of /")

	packed.AddCommand(inspect, install, upgrade, statusCmd, uninstallCmd)
	return packed
}

// runLoad builds a packed install or upgrade command's RunE, the two names an operator gives to load.
func runLoad(self string, root *string, set *[]string, run mode) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, _ []string) error {
		values, err := parseSet(*set)
		if err != nil {
			return err
		}
		work := report.New(command.ErrOrStderr())
		return withPayload(self, func(dir string) error {
			return load(command.Context(), command.OutOrStdout(), work,
				machines, machine.NewShell(machine.Sh), dir, *root, values, false, run)
		})
	}
}

// bindRootFlag gives command the hidden --root flag every verb shares.
func bindRootFlag(command *cobra.Command, root *string, help string) {
	command.Flags().StringVar(root, "root", "/", help)
	command.Flags().MarkHidden("root")
}

// withPayload lays the carried bundle down in a working directory and hands it to run.
func withPayload(self string, run func(dir string) error) error {
	dir, err := os.MkdirTemp("", "vessel-bundle-*")
	if err != nil {
		return fmt.Errorf("create a working directory: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := installer.Unpack(self, dir); err != nil {
		return err
	}
	return run(dir)
}
