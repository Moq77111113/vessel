package cli

import (
	"aead.dev/minisign"

	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/registry"
	"github.com/Moq77111113/vessel/internal/report"
)

// puller is what resolveAll needs from a registry: resolve a reference to a digest, then pull
// the manifest that digest names. *registry.Client satisfies it.
type puller interface {
	Resolve(ctx context.Context, ref descriptor.Ref, platform string) (string, error)
	Image(ctx context.Context, ref descriptor.Ref) (v1.Image, error)
}

// ErrTargetCollides says a delivery file claims a path a unit already occupies.
var ErrTargetCollides = errors.New("a file target collides with a unit")

// ErrTargetDuplicate says two delivery files claim the same path.
var ErrTargetDuplicate = errors.New("a file target is declared twice")

// ErrSecretNoUnitReads says a delivery declares a secret no unit ever reads. The podman secret
// vessel creates carries the variable name, and Secret= is the only way a container sees it.
var ErrSecretNoUnitReads = errors.New("no unit reads this secret with a Secret= key")

// ErrTargetInUnitDirectory says a delivery file lands where the reader writes its units. A file
// carried there is never read as a unit, so its image is never pinned to a digest.
var ErrTargetInUnitDirectory = errors.New("a file target lands in the unit directory")

func newLink() *cobra.Command {
	var out, platform, name, version, key string

	link := &cobra.Command{
		Use:   "link <source>",
		Short: "Resolve a descriptor to digests and write a signed bundle",
		Long: "Reads the descriptor in <source>, resolves every image reference to an\n" +
			"immutable digest, and writes a bundle that carries the images and the\n" +
			"patched descriptor files.",
		Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			work := report.New(command.ErrOrStderr())
			summary := report.New(command.OutOrStdout())
			return link(command.Context(), work, summary, args[0], out, platform, name, version, key)
		},
	}
	flags := link.Flags()
	flags.StringVarP(&out, "out", "o", "", "directory the bundle is written to")
	flags.StringVar(&platform, "platform", "linux/amd64", "platform every reference resolves for")
	flags.StringVar(&name, "name", "", "bundle name")
	flags.StringVar(&version, "version", "", "bundle version")
	flags.StringVar(&key, "key", "", "minisign private key file")
	link.MarkFlagRequired("out")
	return link
}

func link(ctx context.Context, work, summary report.Report, source, out, platform, name, version, key string) error {
	source = filepath.Clean(source)
	definition, err := delivery.Read(os.DirFS(source))
	if err != nil && !errors.Is(err, delivery.ErrNoDelivery) {
		return err
	}
	if name == "" {
		name = definition.Name
	}
	if err := delivery.CheckName(name); err != nil {
		return err
	}
	if version == "" {
		version = definition.Version
	}
	units := source
	if definition.Units != "" {
		units = filepath.Join(source, definition.Units)
	}
	dir := os.DirFS(units)
	reader, err := descriptor.Pick(readers, dir)
	if err != nil {
		return err
	}
	manifest, err := reader.Read(dir)
	if err != nil {
		return err
	}
	plain, err := carryFiles(source, definition.Files)
	if err != nil {
		return err
	}
	if err := checkNoCollision(manifest.Files, plain); err != nil {
		return err
	}
	if err := checkOutsideTheUnitDirectories(reader, plain); err != nil {
		return err
	}
	if err := checkEverySecretIsRead(reader.Requires(manifest.Files), definition.Variables); err != nil {
		return err
	}
	manifest.Files = append(manifest.Files, plain...)
	if err := delivery.CheckMarkers(manifest.Files, definition.Variables); err != nil {
		return err
	}

	layout, err := os.MkdirTemp("", "vessel-layout-*")
	if err != nil {
		return fmt.Errorf("create a working directory: %w", err)
	}
	defer os.RemoveAll(layout)

	digests, err := resolveAll(ctx, work, registry.New(), manifest.Relocs, platform, layout)
	if err != nil {
		return err
	}
	files, err := bundle.Patch(manifest, digests)
	if err != nil {
		return err
	}
	contents := bundle.Contents{
		Layout: layout,
		Files:  files,
		Config: bundle.Config{
			Name:      name,
			Version:   version,
			Reader:    reader.Name(),
			Platform:  platform,
			Images:    imagesOf(digests),
			Variables: definition.Variables,
			Actions:   definition.Actions,
		},
	}
	if err := bundle.Write(out, contents); err != nil {
		return err
	}
	if err := signBundle(out, key); err != nil {
		return err
	}
	summary.Line("Finished", fmt.Sprintf("%d images, %d files, bundle in %s", len(digests), len(files), out))
	return nil
}

// resolveAll pulls every relocation's image into the layout, announcing each before the call it
// precedes so a long pull shows it is moving rather than going silent until it returns.
func resolveAll(ctx context.Context, work report.Report, client puller, relocs []descriptor.Relocation, platform, layout string) (map[string]string, error) {
	digests := make(map[string]string, len(relocs))
	for _, reloc := range relocs {
		tag := reloc.Ref.String()
		if _, seen := digests[tag]; seen {
			continue
		}
		work.Line("Resolving", tag)
		digest, err := client.Resolve(ctx, reloc.Ref, platform)
		if err != nil {
			return nil, err
		}
		ref := reloc.Ref.WithDigest(digest)
		work.Line("Pulling", ref.String())
		image, err := client.Image(ctx, ref)
		if err != nil {
			return nil, err
		}
		if err := registry.WriteLayout(layout, ref, image); err != nil {
			return nil, err
		}
		digests[tag] = digest
	}
	return digests, nil
}

func signBundle(dir, keyPath string) error {
	if keyPath == "" {
		return nil
	}
	key, err := readPrivateKey(keyPath)
	if err != nil {
		return err
	}
	root, err := bundle.RootBytes(dir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, bundle.SignatureName)
	if err := os.WriteFile(path, attest.Sign(key, root), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
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

// carryFiles reads the plain files a delivery declares, keyed by the path they take under the root.
func carryFiles(source string, mappings []delivery.File) ([]descriptor.File, error) {
	files := make([]descriptor.File, 0, len(mappings))
	for _, mapping := range mappings {
		data, err := os.ReadFile(filepath.Join(source, mapping.Source))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", mapping.Source, err)
		}
		files = append(files, descriptor.File{Path: strings.TrimPrefix(mapping.Target, "/"), Data: data})
	}
	return files, nil
}

// checkNoCollision refuses a plain file whose path a unit already occupies, or that another
// plain file already claims.
func checkNoCollision(units, files []descriptor.File) error {
	unitTargets := make(map[string]bool, len(units))
	for _, unit := range units {
		unitTargets[unit.Path] = true
	}
	targets := make(map[string]bool, len(files))
	for _, file := range files {
		if unitTargets[file.Path] {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetCollides)
		}
		if targets[file.Path] {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetDuplicate)
		}
		targets[file.Path] = true
	}
	return nil
}

// checkOutsideTheUnitDirectories refuses a plain file that lands where the reader writes its
// units: link never reads it as a unit, so nothing pins the image it may name.
func checkOutsideTheUnitDirectories(reader descriptor.Reader, files []descriptor.File) error {
	for _, file := range files {
		if reader.Owns(file.Path) {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetInUnitDirectory)
		}
	}
	return nil
}

// checkEverySecretIsRead refuses a secret variable no unit reads: vessel creates the podman secret
// under the variable name, and a unit reaches it with a Secret= key naming that same name.
func checkEverySecretIsRead(required []string, variables []delivery.Variable) error {
	for _, name := range delivery.SecretNames(variables) {
		if !slices.Contains(required, name) {
			return fmt.Errorf("%s: %w", name, ErrSecretNoUnitReads)
		}
	}
	return nil
}

// imagesOf turns the resolution map into the ordered list the config carries.
func imagesOf(digests map[string]string) []bundle.Image {
	images := make([]bundle.Image, 0, len(digests))
	for ref, digest := range digests {
		images = append(images, bundle.Image{Ref: ref, Digest: digest})
	}
	sort.Slice(images, func(a, b int) bool { return images[a].Ref < images[b].Ref })
	return images
}
