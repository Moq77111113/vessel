package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
)

func newInspect() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <bundle>",
		Short: "Print what a bundle carries, without touching anything",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return inspectBundle(command.OutOrStdout(), args[0])
		},
	}
}

func inspectBundle(out io.Writer, dir string) error {
	opened, err := bundle.Open(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s %s, read by %s, resolved for %s on %s\n",
		opened.Config.Name, opened.Config.Version, opened.Config.Reader,
		opened.Config.Platform, opened.Config.Time)
	for _, image := range opened.Config.Images {
		fmt.Fprintf(out, "image %s %s\n", image.Ref, image.Digest)
	}
	for _, file := range opened.Files {
		fmt.Fprintf(out, "file  %s %d bytes\n", file.Path, len(file.Data))
	}
	return nil
}
