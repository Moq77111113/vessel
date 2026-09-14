package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/descriptor"
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
	return newLoadCommand(modeInstall, "install <bundle>", "Install a bundle on this machine",
		"Checks the machine, verifies the bundle, puts its images into local storage and\n"+
			"its files on disk, removes any file a previous version put that this one does\n"+
			"not carry, then starts the services once.\n\n"+
			"For an operator, prefer an executable made by build: it needs no vessel binary.")
}

// newUpgrade is install restricted to a machine that already holds a record of this delivery.
func newUpgrade() *cobra.Command {
	return newLoadCommand(modeUpgrade, "upgrade <bundle>", "Install a newer version of a bundle on this machine",
		"Does what install does, and refuses a machine that holds no record of this\n"+
			"delivery: run install there instead.")
}

// mode says whether load runs as an install, which accepts a bare machine, or an upgrade,
// which requires an existing record to move forward from.
type mode int

const (
	modeInstall mode = iota
	modeUpgrade
)

// newLoadCommand builds an install or upgrade command, the two names an operator gives to load.
func newLoadCommand(run mode, use, short, long string) *cobra.Command {
	var root string
	var set []string

	command := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.ExactArgs(1),
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
				machines, machine.NewShell(machine.Sh), args[0], root, values, true, run)
		},
	}
	bindRootFlag(command, &root, "install under this directory instead of /")
	command.Flags().StringArrayVar(&set, "set", nil, "answer a variable: --set NAME=value")
	return command
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
	set map[string]string, verify bool, run mode) error {
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

	records := machine.NewRecords(root, artifact.Config.Name)
	current, found, err := records.Read()
	if err != nil {
		return err
	}
	if run == modeUpgrade && !found {
		return fmt.Errorf("%s: %w, run install instead", artifact.Config.Name, ErrNoRecord)
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

	record := machine.Record{
		Name:    artifact.Config.Name,
		Version: artifact.Config.Version,
		Machine: artifact.Config.Reader,
		Root:    artifact.Root,
		Files:   entriesOf(files),
		Images:  digestsOf(artifact.Config.Images),
		Secrets: delivery.SecretNames(artifact.Config.Variables),
		Start:   time.Now().UTC(),
	}
	if err := records.Write(record); err != nil {
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
	if found {
		if err := dropStale(ctx, work, kind, tree, current.Files, record.Files); err != nil {
			return err
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

	reportMissingSecrets(ctx, out, kind, kind.Requires(files))
	if err := kind.Start(ctx, files); err != nil {
		return err
	}
	if err := checkServicesUp(ctx, kind, files); err != nil {
		return err
	}

	record.End = time.Now().UTC()
	if err := records.Write(record); err != nil {
		return err
	}

	report.New(out).Line("Finished", fmt.Sprintf("%s %s installed and running: %d images, %d of %d files changed",
		artifact.Config.Name, artifact.Config.Version, len(images), changes, len(files)))
	return nil
}

// ErrServiceIsDown says a service the install started did not come up.
var ErrServiceIsDown = errors.New("did not start")

// checkServicesUp fails naming every service the machine reports as down.
func checkServicesUp(ctx context.Context, kind machine.Target, files []descriptor.File) error {
	services, err := kind.Services(ctx, files)
	if err != nil {
		return err
	}
	var down []string
	for _, service := range services {
		if !service.Running {
			down = append(down, service.Name)
		}
	}
	if len(down) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %w", strings.Join(down, ", "), ErrServiceIsDown)
}

// entriesOf names every file this install puts down, with the digest of what it wrote.
func entriesOf(files []descriptor.File) []machine.Entry {
	entries := make([]machine.Entry, 0, len(files))
	for _, file := range files {
		entries = append(entries, machine.Entry{
			Path:   file.Path,
			Digest: machine.DigestOf(file.Data),
		})
	}
	return entries
}

// onlyIn names the files the previous version put and the new one does not carry.
func onlyIn(previous, next []machine.Entry) []string {
	carried := make(map[string]bool, len(next))
	for _, entry := range next {
		carried[entry.Path] = true
	}
	var stale []string
	for _, entry := range previous {
		if !carried[entry.Path] {
			stale = append(stale, entry.Path)
		}
	}
	slices.Sort(stale)
	return stale
}

// dropStale stops the services a file the new version no longer carries used to generate,
// then removes that file, so nothing on disk still points to a service nothing can stop again.
func dropStale(ctx context.Context, work report.Report, kind machine.Target, tree *machine.Tree,
	previous, next []machine.Entry) error {
	stale := onlyIn(previous, next)
	if len(stale) == 0 {
		return nil
	}
	dropped := make([]descriptor.File, 0, len(stale))
	for _, path := range stale {
		dropped = append(dropped, descriptor.File{Path: path})
	}
	if err := kind.Stop(ctx, dropped); err != nil {
		return err
	}
	for _, path := range stale {
		removed, err := tree.Remove(path)
		if err != nil {
			return err
		}
		if removed {
			work.Line("Removing", path)
		}
	}
	return nil
}

// digestsOf names the digest an install resolved each image to.
func digestsOf(images []bundle.Image) []string {
	digests := make([]string, 0, len(images))
	for _, image := range images {
		digests = append(digests, image.Digest)
	}
	return digests
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
