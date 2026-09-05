package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/target"
)

// publicKey is stamped in at build time and is the only key a bundle is trusted against.
//
//	go build -ldflags "-X github.com/Moq77111113/vessel/internal/cli.publicKey=$(cat vessel.pub)"
var publicKey string

// Errors load reports before it looks at a bundle.
var (
	ErrNoPublicKey = errors.New("this build carries no public key, it cannot verify a signed bundle")
	ErrNoBundle    = errors.New("is not a vessel bundle")
)

func newLoad() *cobra.Command {
	var root string

	load := &cobra.Command{
		Use:   "load <bundle>",
		Short: "Install a bundle on this machine",
		Long: "Checks the machine, verifies the bundle, puts its images into local storage\n" +
			"and its files on disk, then prints the command that starts everything.\n\n" +
			"For an operator, prefer an executable made by pack: it needs no vessel binary.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !bundle.IsBundle(args[0]) {
				return fmt.Errorf("%s %w", args[0], ErrNoBundle)
			}
			return load(command.Context(), command.OutOrStdout(), args[0], root, true)
		},
	}
	load.Flags().StringVar(&root, "root", "/", "install under this directory instead of /")
	load.Flags().MarkHidden("root")
	return load
}

// load installs a bundle. verify is false when the bundle rode inside this executable:
// a binary cannot vouch for the payload it carries, so the operator checks the file itself.
func load(ctx context.Context, out io.Writer, dir, root string, verify bool) error {
	loader := target.NewLoader(target.Exec)
	if err := target.Preflight(ctx, loader, root); err != nil {
		return err
	}
	if verify {
		if err := verifyBundle(dir); err != nil {
			return err
		}
	}
	opened, err := bundle.Open(dir)
	if err != nil {
		return err
	}
	layout, err := target.OpenLayout(opened.LayoutDir)
	if err != nil {
		return err
	}
	images, err := loader.Load(ctx, layout)
	if err != nil {
		return err
	}

	tree := target.NewTree(root)
	changes := 0
	for _, file := range opened.Files {
		changed, err := tree.Write(file)
		if err != nil {
			return err
		}
		if changed {
			changes++
		}
	}

	fmt.Fprintf(out, "%s %s installed: %d images, %d of %d files changed\n",
		opened.Config.Name, opened.Config.Version, len(images), changes, len(opened.Files))
	reader, err := descriptor.ByName(readers, opened.Config.Reader)
	if err != nil {
		return err
	}
	reportMissingSecrets(ctx, out, loader, reader.Requires(opened.Files))

	commands := reader.Start(opened.Files)
	if len(commands) == 0 {
		return nil
	}
	fmt.Fprintf(out, "\nStart it:\n  %s\n", strings.Join(commands, "\n  "))
	return nil
}

func verifyBundle(dir string) error {
	root, err := bundle.RootBytes(dir)
	if err != nil {
		return err
	}
	signature, err := os.ReadFile(filepath.Join(dir, bundle.SignatureName))
	if err != nil {
		if publicKey == "" {
			return nil
		}
		return fmt.Errorf("read the bundle signature: %w", err)
	}
	if publicKey == "" {
		return ErrNoPublicKey
	}
	verifier, err := attest.NewVerifier(publicKey)
	if err != nil {
		return err
	}
	return verifier.Verify(root, signature)
}

// reportMissingSecrets names the secrets the units expect and the machine does not hold.
// It never fails the install: the operator may be about to create them.
func reportMissingSecrets(ctx context.Context, out io.Writer, loader *target.Loader, wanted []string) {
	if len(wanted) == 0 {
		return
	}
	held, err := loader.Secrets(ctx)
	if err != nil {
		return
	}
	machine := make(map[string]bool, len(held))
	for _, name := range held {
		machine[name] = true
	}
	var missing []string
	for _, name := range wanted {
		if !machine[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return
	}
	fmt.Fprintf(out, "\nThese secrets are not on this machine yet, the units need them:\n")
	for _, name := range missing {
		fmt.Fprintf(out, "  podman secret create %s <file>\n", name)
	}
}
