package cli

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/link"
	"github.com/Moq77111113/vessel/internal/report"
)

func newBuild() *cobra.Command {
	var job link.Job

	command := &cobra.Command{
		Use:   "build <source>",
		Short: "Resolve a descriptor to digests and write one executable",
		Long: "Reads the descriptor in <source>, resolves every image reference to an\n" +
			"immutable digest, and writes a single file the target machine runs.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			job.Source = args[0]
			return link.Build(c.Context(), report.New(c.ErrOrStderr()), c.OutOrStdout(), machines, job)
		},
	}
	flags := command.Flags()
	flags.StringVarP(&job.Out, "out", "o", "", "the executable to write")
	flags.StringVar(&job.Layout, "layout", "", "also write the OCI layout here, for a registry")
	flags.StringVar(&job.Platform, "platform", "", "platform every reference resolves for (default linux/amd64)")
	flags.StringVar(&job.Name, "name", "", "delivery name")
	flags.StringVar(&job.Version, "version", "", "delivery version")
	flags.StringArrayVar(&job.Evidence, "evidence", nil, "attach an SBOM or a report made from the bundle source")
	flags.StringVar(&job.Key, "key", "", "PKCS#8 PEM ECDSA P-256 private key the release is signed with")
	flags.BoolVar(&job.InsecureUnsigned, "insecure-unsigned", false,
		"build without a signature; development use only")
	command.MarkFlagRequired("out")
	return command
}
