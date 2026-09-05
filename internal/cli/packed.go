package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/installer"
)

// newPacked is the command tree of an executable that carries its own bundle.
// It has one job, so it offers two verbs and no way to build anything.
func newPacked(self string) *cobra.Command {
	var root string

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
		RunE: func(command *cobra.Command, _ []string) error {
			return withPayload(self, func(dir string) error {
				return load(command.Context(), command.OutOrStdout(), dir, root, false)
			})
		},
	}
	install.Flags().StringVar(&root, "root", "/", "install under this directory instead of /")
	install.Flags().MarkHidden("root")

	packed.AddCommand(inspect, install)
	return packed
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
