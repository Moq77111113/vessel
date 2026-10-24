package main

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/build"
	"github.com/Moq77111113/vessel/internal/report"
)

func newBuild() *cobra.Command {
	var req build.Request

	command := &cobra.Command{
		Use:   "build <source>",
		Short: "Resolve a descriptor to digests and write one executable",
		Long: "Reads the descriptor in <source>, resolves every image reference to an\n" +
			"immutable digest, and writes a single file the target machine runs.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			req.Source = args[0]
			req.Work = report.New(c.ErrOrStderr())
			req.Stdout = c.OutOrStdout()
			_, err := build.Build{Drivers: machines}.Run(c.Context(), req)
			return err
		},
	}
	flags := command.Flags()
	flags.StringVarP(&req.Out, "out", "o", "", "the executable to write")
	flags.StringVar(&req.Layout, "layout", "", "also write the OCI layout here, for a registry")
	flags.StringVar(&req.Platform, "platform", "", "platform every reference resolves for (default linux/amd64)")
	flags.StringVar(&req.Name, "name", "", "delivery name")
	flags.StringVar(&req.Version, "version", "", "delivery version")
	flags.StringArrayVar(&req.Evidence, "evidence", nil, "attach an SBOM or a report made from the bundle source")
	flags.StringVar(&req.Key, "key", "", "PKCS#8 PEM ECDSA P-256 private key the release is signed with")
	flags.BoolVar(&req.InsecureUnsigned, "insecure-unsigned", false,
		"build without a signature; development use only")
	command.MarkFlagRequired("out")
	return command
}
