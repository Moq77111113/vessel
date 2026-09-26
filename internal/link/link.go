// Package link resolves a delivery to digests and writes the bundle that carries it.
package link

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/registry"
	"github.com/Moq77111113/vessel/internal/report"
)

// puller is what resolveAll needs from a registry: resolve a reference to a digest, then pull
// the manifest that digest names. *registry.Client satisfies it.
type puller interface {
	Resolve(ctx context.Context, ref descriptor.Ref, platform string) (string, error)
	Image(ctx context.Context, ref descriptor.Ref) (v1.Image, error)
}

// defaultPlatform is what every reference resolves for when the job names no platform.
const defaultPlatform = "linux/amd64"

// Link resolves the delivery the job names to digests and writes its bundle in out.
func Link(ctx context.Context, work, summary report.Report, kinds []machine.Machine, job Job, out string) error {
	source, err := os.OpenRoot(job.Source)
	if err != nil {
		return fmt.Errorf("open %s: %w", job.Source, err)
	}
	defer source.Close()
	definition, err := descriptor.Read(source.FS())
	if err != nil && !errors.Is(err, descriptor.ErrNoDelivery) {
		return err
	}
	name := job.Name
	if name == "" {
		name = definition.Name
	}
	if err := descriptor.CheckName(name); err != nil {
		return err
	}
	version := job.Version
	if version == "" {
		version = definition.Version
	}
	platform := job.Platform
	if platform == "" {
		platform = defaultPlatform
	}
	dir := source.FS()
	if definition.Units != "" {
		if dir, err = fs.Sub(dir, definition.Units); err != nil {
			return err
		}
	}
	kind, err := machine.Pick(kinds, dir)
	if err != nil {
		return err
	}
	manifest, err := kind.Read(dir)
	if err != nil {
		return err
	}
	plain, err := carryFiles(source.FS(), definition.Files)
	if err != nil {
		return err
	}
	if err := checkNoCollision(manifest.Files, plain); err != nil {
		return err
	}
	if err := checkOutsideTheUnitDirectories(kind, plain); err != nil {
		return err
	}
	if err := checkEverySecretIsRead(kind.Requires(manifest.Files), definition.Variables); err != nil {
		return err
	}
	manifest.Files = append(manifest.Files, plain...)
	if err := descriptor.CheckMarkers(manifest.Files, definition.Variables); err != nil {
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
			Machine:   kind.Name(),
			Platform:  platform,
			Images:    imagesOf(digests),
			Variables: definition.Variables,
			Actions:   definition.Actions,
			Insecure:  job.InsecureUnsigned,
		},
	}
	if err := bundle.Write(out, contents); err != nil {
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

// carryFiles reads the plain files a delivery declares, keyed by the path they take under the root.
func carryFiles(source fs.FS, mappings []descriptor.Mapping) ([]descriptor.File, error) {
	files := make([]descriptor.File, 0, len(mappings))
	for _, mapping := range mappings {
		data, err := fs.ReadFile(source, mapping.Source)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", mapping.Source, err)
		}
		files = append(files, descriptor.File{Path: strings.TrimPrefix(mapping.Target, "/"), Data: data})
	}
	return files, nil
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
