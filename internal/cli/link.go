package cli

import (
	"aead.dev/minisign"

	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/registry"
)

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
	dir := os.DirFS(source)
	reader, err := descriptor.Pick(readers, dir)
	if err != nil {
		return err
	}
	manifest, err := reader.Read(dir)
	if err != nil {
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
			Name:     name,
			Version:  version,
			Reader:   reader.Name(),
			Platform: platform,
			Time:     time.Now().UTC().Format(time.RFC3339),
			Images:   imagesOf(digests),
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

// imagesOf turns the resolution map into the ordered list the config carries.
func imagesOf(digests map[string]string) []bundle.Image {
	images := make([]bundle.Image, 0, len(digests))
	for ref, digest := range digests {
		images = append(images, bundle.Image{Ref: ref, Digest: digest})
	}
	sort.Slice(images, func(a, b int) bool { return images[a].Ref < images[b].Ref })
	return images
}
