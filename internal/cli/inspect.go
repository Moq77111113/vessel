package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
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
	artifact, err := bundle.Open(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s %s, read by %s, resolved for %s\n",
		artifact.Config.Name, artifact.Config.Version, artifact.Config.Reader, artifact.Config.Platform)
	lines := report.New(out)
	for _, image := range artifact.Config.Images {
		lines.Line("Image", fmt.Sprintf("%s %s", image.Ref, image.Digest))
	}
	for _, file := range artifact.Files {
		lines.Line("File", fmt.Sprintf("%s %d bytes", file.Path, len(file.Data)))
	}
	return nil
}
