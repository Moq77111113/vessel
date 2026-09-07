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
	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/target"
)

// publicKey is stamped in at build time and is the only key a bundle is trusted against.
//
//	go build -ldflags "-X github.com/Moq77111113/vessel/internal/cli.publicKey=$(cat vessel.pub)"
var publicKey string

// Errors load reports before it looks at a bundle.
var (
	ErrNoPublicKey     = errors.New("this build carries no public key, it cannot verify a signed bundle")
	ErrNoBundle        = errors.New("is not a vessel bundle")
	ErrBundleHasNoName = errors.New("this bundle carries no name, link it again with this vessel")
)

func newLoad() *cobra.Command {
	var root string
	var set []string

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
			values, err := parseSet(set)
			if err != nil {
				return err
			}
			m := machine{loader: target.NewLoader(target.Exec), shell: target.NewShell(target.Sh)}
			return load(command.Context(), command.OutOrStdout(), command.InOrStdin(),
				m, args[0], root, values, true)
		},
	}
	load.Flags().StringVar(&root, "root", "/", "install under this directory instead of /")
	load.Flags().MarkHidden("root")
	load.Flags().StringArrayVar(&set, "set", nil, "answer a variable: --set NAME=value")
	return load
}

// machine is the two channels load reaches this machine through: podman and the shell.
// The composition root builds them; load only uses what it is given, so a test drives
// both without podman.
type machine struct {
	loader *target.Loader
	shell  *target.Shell
}

// Errors a --set flag draws before load looks at a bundle.
var (
	ErrBadSet     = errors.New("is not NAME=value")
	ErrSetIsEmpty = errors.New("gives no value, and a value is never empty")
)

// parseSet turns a repeated --set NAME=value flag into the values it names.
func parseSet(pairs []string) (map[string]string, error) {
	values := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		name, value, ok := strings.Cut(pair, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("--set %q %w", pair, ErrBadSet)
		}
		if value == "" {
			return nil, fmt.Errorf("--set %s %w", name, ErrSetIsEmpty)
		}
		values[name] = value
	}
	return values, nil
}

// load installs a bundle. verify is false when the bundle rode inside this executable:
// a binary cannot vouch for the payload it carries, so the operator checks the file itself.
func load(ctx context.Context, out io.Writer, in io.Reader, m machine, dir, root string,
	set map[string]string, verify bool) error {
	loader, shell := m.loader, m.shell
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
	if opened.Config.Name == "" {
		return fmt.Errorf("%s: %w", dir, ErrBundleHasNoName)
	}

	held, err := loader.Secrets(ctx)
	if err != nil {
		return err
	}
	values := target.NewValues(root, opened.Config.Name)
	resolver := target.NewResolver(values, shell, set, held, out, in)
	resolution, err := resolver.Resolve(ctx, opened.Config.Variables)
	if err != nil {
		return err
	}
	files, err := delivery.Substitute(opened.Files, resolution.Values,
		delivery.SecretNames(opened.Config.Variables))
	if err != nil {
		return err
	}

	for _, action := range opened.Config.Actions {
		if err := shell.Do(ctx, action); err != nil {
			return err
		}
	}

	tree := target.NewTree(root)
	changes := 0
	for _, file := range files {
		changed, err := tree.Write(file)
		if err != nil {
			return err
		}
		if changed {
			changes++
		}
	}
	if err := values.Write(resolution.Values); err != nil {
		return err
	}
	for name, value := range resolution.Secrets {
		if err := loader.CreateSecret(ctx, name, value); err != nil {
			return err
		}
	}

	layout, err := target.OpenLayout(opened.LayoutDir)
	if err != nil {
		return err
	}
	images, err := loader.Load(ctx, layout)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "%s %s installed: %d images, %d of %d files changed\n",
		opened.Config.Name, opened.Config.Version, len(images), changes, len(files))
	reader, err := descriptor.ByName(readers, opened.Config.Reader)
	if err != nil {
		return err
	}
	reportMissingSecrets(ctx, out, loader, reader.Requires(files))

	commands := reader.Start(files)
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
