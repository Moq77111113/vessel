package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/target"
)

// newPacked is the command tree of an executable that carries its own bundle.
// It has one job, so it offers two verbs and no way to build anything.
func newPacked(self string) *cobra.Command {
	var root string
	var set []string

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
			values, err := parseSet(set)
			if err != nil {
				return err
			}
			m := machine{loader: target.NewLoader(target.Exec), shell: target.NewShell(target.Sh)}
			work := report.New(command.ErrOrStderr())
			return withPayload(self, func(dir string) error {
				return load(command.Context(), command.OutOrStdout(), work, command.InOrStdin(),
					m, dir, root, values, false)
			})
		},
	}
	install.Flags().StringVar(&root, "root", "/", "install under this directory instead of /")
	install.Flags().MarkHidden("root")
	install.Flags().StringArrayVar(&set, "set", nil, "answer a variable: --set NAME=value")

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
