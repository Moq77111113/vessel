package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"aead.dev/minisign"
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/report"
)

func newBuild() *cobra.Command {
	var out, layout, platform, name, version, key string

	command := &cobra.Command{
		Use:   "build <source>",
		Short: "Resolve a descriptor to digests and write one executable",
		Long: "Reads the descriptor in <source>, resolves every image reference to an\n" +
			"immutable digest, and writes a single file the target machine runs.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			work := report.New(c.ErrOrStderr())
			return build(c.Context(), work, c.OutOrStdout(), args[0], out, layout, platform, name, version, key)
		},
	}
	flags := command.Flags()
	flags.StringVarP(&out, "out", "o", "", "the executable to write")
	flags.StringVar(&layout, "layout", "", "also write the OCI layout here, for a registry")
	flags.StringVar(&platform, "platform", "linux/amd64", "platform every reference resolves for")
	flags.StringVar(&name, "name", "", "delivery name")
	flags.StringVar(&version, "version", "", "delivery version")
	flags.StringVar(&key, "key", "", "minisign private key file")
	command.MarkFlagRequired("out")
	return command
}

// build resolves the descriptor into a bundle, then folds that bundle into one executable.
func build(ctx context.Context, work report.Report, stdout io.Writer, source, out, layout, platform, name, version, key string) error {
	dir := layout
	if dir == "" {
		temp, err := os.MkdirTemp("", "vessel-bundle-*")
		if err != nil {
			return fmt.Errorf("create a working directory: %w", err)
		}
		defer os.RemoveAll(temp)
		dir = temp
	}

	if err := link(ctx, work, report.New(io.Discard), source, dir, platform, name, version, key); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find this binary: %w", err)
	}
	stub, err := os.Open(self)
	if err != nil {
		return fmt.Errorf("read this binary: %w", err)
	}
	defer stub.Close()

	if err := installer.Pack(stub, dir, out, work); err != nil {
		return err
	}
	if err := signFile(out, key); err != nil {
		return err
	}
	info, err := os.Stat(out)
	if err != nil {
		return fmt.Errorf("read %s: %w", out, err)
	}
	report.New(stdout).Line("Finished", fmt.Sprintf("%s, %d MB, run it on the target machine", out, info.Size()/(1<<20)))
	if key != "" {
		fmt.Fprintf(stdout, "%s.minisig, ship it alongside\n", out)
		return nil
	}
	fmt.Fprintf(stdout, "unsigned: the operator has no way to tell this file is yours.\n"+
		"Sign it with --key, or print `sha256sum %s` on the install sheet.\n", filepath.Base(out))
	return nil
}

// signFile writes a detached minisign signature beside path, the way the minisign tool does.
func signFile(path, keyPath string) error {
	if keyPath == "" {
		return nil
	}
	key, err := readPrivateKey(keyPath)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	signature := path + ".minisig"
	if err := os.WriteFile(signature, attest.Sign(key, body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", signature, err)
	}
	return nil
}

// readPrivateKey opens a minisign private key, with VESSEL_KEY_PASSWORD when it has one.
func readPrivateKey(path string) (minisign.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return minisign.PrivateKey{}, fmt.Errorf("read the private key: %w", err)
	}
	key, err := attest.ReadKey(data, os.Getenv("VESSEL_KEY_PASSWORD"))
	if err != nil {
		return minisign.PrivateKey{}, fmt.Errorf("%s: %w", path, err)
	}
	return key, nil
}
