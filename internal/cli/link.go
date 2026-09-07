package cli

import (
	"aead.dev/minisign"

	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/registry"
)

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
			return link(command.Context(), command.OutOrStdout(), args[0], out, platform, name, version, key)
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

func link(ctx context.Context, out2 io.Writer, source, out, platform, name, version, key string) error {
	source = filepath.Clean(source)
	declared, err := delivery.Read(os.DirFS(source))
	if err != nil && !errors.Is(err, delivery.ErrNoDelivery) {
		return err
	}
	if name == "" {
		name = declared.Name
	}
	if err := delivery.CheckName(name); err != nil {
		return err
	}
	if version == "" {
		version = declared.Version
	}
	units := source
	if declared.Units != "" {
		units = filepath.Join(source, declared.Units)
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
	carried, err := carryFiles(source, declared.Files)
	if err != nil {
		return err
	}
	if err := checkNoCollision(manifest.Files, carried); err != nil {
		return err
	}
	if err := checkOutsideTheUnitDirectories(reader, carried); err != nil {
		return err
	}
	if err := checkEverySecretIsRead(reader.Requires(manifest.Files), declared.Variables); err != nil {
		return err
	}
	manifest.Files = append(manifest.Files, carried...)
	if err := delivery.CheckMarkers(manifest.Files, declared.Variables); err != nil {
		return err
	}

	layout, err := os.MkdirTemp("", "vessel-layout-*")
	if err != nil {
		return fmt.Errorf("create a working directory: %w", err)
	}
	defer os.RemoveAll(layout)

	digests, err := resolveAll(ctx, registry.New(), manifest.Relocs, platform, layout)
	if err != nil {
		return err
	}
	files, err := bundle.Patch(manifest, digests)
	if err != nil {
		return err
	}
	writing := bundle.Writing{
		Layout: layout,
		Files:  files,
		Config: bundle.Config{
			Name:      name,
			Version:   version,
			Reader:    reader.Name(),
			Platform:  platform,
			Time:      time.Now().UTC().Format(time.RFC3339),
			Images:    imagesOf(digests),
			Variables: declared.Variables,
			Actions:   declared.Actions,
		},
	}
	if err := bundle.Write(out, writing); err != nil {
		return err
	}
	if err := signBundle(out, key); err != nil {
		return err
	}
	for _, image := range writing.Config.Images {
		fmt.Fprintf(out2, "%s %s\n", image.Ref, image.Digest)
	}
	fmt.Fprintf(out2, "%d images, %d files, bundle in %s\n", len(digests), len(files), out)
	return nil
}

func resolveAll(ctx context.Context, client *registry.Client, relocs []descriptor.Relocation, platform, layout string) (map[string]string, error) {
	digests := make(map[string]string, len(relocs))
	for _, reloc := range relocs {
		ref := reloc.Ref.String()
		if _, seen := digests[ref]; seen {
			continue
		}
		digest, err := client.Resolve(ctx, reloc.Ref, platform)
		if err != nil {
			return nil, err
		}
		if err := client.Fetch(ctx, reloc.Ref.WithDigest(digest), layout); err != nil {
			return nil, err
		}
		digests[ref] = digest
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
func carryFiles(source string, declared []delivery.File) ([]descriptor.File, error) {
	files := make([]descriptor.File, 0, len(declared))
	for _, file := range declared {
		data, err := os.ReadFile(filepath.Join(source, file.Source))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file.Source, err)
		}
		files = append(files, descriptor.File{Path: strings.TrimPrefix(file.Target, "/"), Data: data})
	}
	return files, nil
}

// checkNoCollision refuses a carried file whose path a unit already occupies, or that another
// carried file already claims.
func checkNoCollision(units, carried []descriptor.File) error {
	occupied := make(map[string]bool, len(units))
	for _, unit := range units {
		occupied[unit.Path] = true
	}
	claimed := make(map[string]bool, len(carried))
	for _, file := range carried {
		if occupied[file.Path] {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetCollides)
		}
		if claimed[file.Path] {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetDuplicate)
		}
		claimed[file.Path] = true
	}
	return nil
}

// checkOutsideTheUnitDirectories refuses a carried file that lands where the reader writes its
// units: link never reads it as a unit, so nothing pins the image it may name.
func checkOutsideTheUnitDirectories(reader descriptor.Reader, carried []descriptor.File) error {
	for _, file := range carried {
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
