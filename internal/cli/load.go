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
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

// publicKey is stamped in at build time and is the only key a bundle is trusted against.
//
//	go build -ldflags "-X github.com/Moq77111113/vessel/internal/cli.publicKey=$(cat vessel.pub)"
var publicKey string

// Errors load reports before it looks at a bundle.
var (
	ErrNoPublicKey     = errors.New("this build carries no public key, it cannot verify a signed bundle")
	ErrNoBundle        = errors.New("is not a vessel bundle")
	ErrBundleHasNoName = errors.New("this bundle carries no name, build it again with this vessel")
)

func newInstall() *cobra.Command {
	var root string
	var set []string

	install := &cobra.Command{
		Use:   "install <bundle>",
		Short: "Install a bundle on this machine",
		Long: "Checks the machine, verifies the bundle, puts its images into local storage\n" +
			"and its files on disk, then starts the services once.\n\n" +
			"For an operator, prefer an executable made by build: it needs no vessel binary.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !bundle.IsBundle(args[0]) {
				return fmt.Errorf("%s %w", args[0], ErrNoBundle)
			}
			values, err := parseSet(set)
			if err != nil {
				return err
			}
			work := report.New(command.ErrOrStderr())
			return load(command.Context(), command.OutOrStdout(), work, command.InOrStdin(),
				machines, machine.NewShell(machine.Sh), args[0], root, values, true)
		},
	}
	install.Flags().StringVar(&root, "root", "/", "install under this directory instead of /")
	install.Flags().MarkHidden("root")
	install.Flags().StringArrayVar(&set, "set", nil, "answer a variable: --set NAME=value")
	return install
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
func load(ctx context.Context, out io.Writer, work report.Report, in io.Reader,
	kinds []machine.Machine, shell *machine.Shell, dir, root string,
	set map[string]string, verify bool) error {
	if verify {
		if err := verifyBundle(dir); err != nil {
			return err
		}
	}
	artifact, err := bundle.Open(dir)
	if err != nil {
		return err
	}
	if artifact.Config.Name == "" {
		return fmt.Errorf("%s: %w", dir, ErrBundleHasNoName)
	}
	kind, err := machine.ByName(kinds, artifact.Config.Reader)
	if err != nil {
		return err
	}
	if err := kind.Check(ctx, root); err != nil {
		return err
	}

	secrets, err := kind.Secrets(ctx)
	if err != nil {
		return err
	}
	site := machine.NewSite(root, artifact.Config.Name)
	values := machine.NewValues(site, shell, set, secrets, out, in)
	resolution, err := values.Resolve(ctx, artifact.Config.Variables)
	if err != nil {
		return err
	}
	files, err := delivery.Substitute(artifact.Files, resolution.Values,
		delivery.SecretNames(artifact.Config.Variables))
	if err != nil {
		return err
	}

	for _, action := range artifact.Config.Actions {
		if err := shell.Do(ctx, action); err != nil {
			return err
		}
	}

	tree := machine.NewTree(root)
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
	if err := site.Write(resolution.Values); err != nil {
		return err
	}
	for name, value := range resolution.Secrets {
		if err := kind.AddSecret(ctx, name, value); err != nil {
			return err
		}
	}

	layout, err := machine.OpenLayout(artifact.LayoutDir)
	if err != nil {
		return err
	}
	images, err := kind.AddImages(ctx, work, layout)
	if err != nil {
		return err
	}

	report.New(out).Line("Finished", fmt.Sprintf("%s %s installed: %d images, %d of %d files changed",
		artifact.Config.Name, artifact.Config.Version, len(images), changes, len(files)))
	reportMissingSecrets(ctx, out, kind, kind.Requires(files))

	return kind.Start(ctx, files)
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
func reportMissingSecrets(ctx context.Context, out io.Writer, target machine.Target, units []string) {
	if len(units) == 0 {
		return
	}
	secrets, err := target.Secrets(ctx)
	if err != nil {
		return
	}
	present := make(map[string]bool, len(secrets))
	for _, name := range secrets {
		present[name] = true
	}
	var missing []string
	for _, name := range units {
		if !present[name] {
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
