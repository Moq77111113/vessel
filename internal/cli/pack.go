package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/report"
)

// ErrNotABundle says the path handed to pack is not a bundle.
var ErrNotABundle = errors.New("is not a vessel bundle")

func newPack() *cobra.Command {
	var out, key string

	pack := &cobra.Command{
		Use:   "pack <bundle>",
		Short: "Turn a bundle into one executable that installs itself",
		Long: "Writes a single file the operator runs. It carries this vessel binary and the\n" +
			"bundle, so the machine needs nothing else.\n\n" +
			"The operator runs an executable off removable media, so sign the result and\n" +
			"have them check it: minisign -Vm <file> -P <key>.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			work := report.New(command.ErrOrStderr())
			return pack(command.OutOrStdout(), work, args[0], out, key)
		},
	}
	pack.Flags().StringVarP(&out, "out", "o", "", "the executable to write")
	pack.Flags().StringVar(&key, "key", "", "minisign private key, to sign the executable")
	pack.MarkFlagRequired("out")
	return pack
}

func pack(out io.Writer, work report.Report, dir, path, key string) error {
	if !bundle.IsBundle(dir) {
		return fmt.Errorf("%s %w", dir, ErrNotABundle)
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

	if err := installer.Pack(stub, dir, path, work); err != nil {
		return err
	}
	if err := signFile(path, key); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	report.New(out).Line("Finished", fmt.Sprintf("%s, %d MB, run it on the target machine", path, info.Size()/(1<<20)))
	if key != "" {
		fmt.Fprintf(out, "%s.minisig, ship it alongside\n", path)
		return nil
	}
	fmt.Fprintf(out, "unsigned: the operator has no way to tell this file is yours.\n"+
		"Sign it with --key, or print `sha256sum %s` on the install sheet.\n", filepath.Base(path))
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
