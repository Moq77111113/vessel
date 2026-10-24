package build

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/Moq77111113/vessel/internal/build/registry"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

type puller interface {
	Resolve(ctx context.Context, ref delivery.Ref, platform string) (string, error)
	Image(ctx context.Context, ref delivery.Ref) (v1.Image, error)
}

const defaultPlatform = "linux/amd64"

func Link(ctx context.Context, work, summary report.Report, kinds []driver.Driver, job Job, out string) error {
	source, err := os.OpenRoot(job.Source)
	if err != nil {
		return fmt.Errorf("open %s: %w", job.Source, err)
	}
	defer source.Close()
	definition, err := delivery.Read(source.FS())
	if err != nil && !errors.Is(err, delivery.ErrNoDelivery) {
		return err
	}
	name := job.Name
	if name == "" {
		name = definition.Name
	}
	if err := delivery.CheckName(name); err != nil {
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
	if definition.Source != "" {
		if dir, err = fs.Sub(dir, definition.Source); err != nil {
			return err
		}
	}
	kind, err := driver.Pick(kinds, dir)
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
	if err := delivery.CheckCollisions(manifest.Files, plain); err != nil {
		return err
	}
	if err := checkOutsideTheDriverDirectories(manifest.Dirs, plain); err != nil {
		return err
	}
	if err := checkEverySecretIsRead(manifest.Secrets, definition.Variables); err != nil {
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
			Machine:   kind.Name(),
			Platform:  platform,
			Images:    imagesOf(digests),
			Services:  manifest.Services,
			Secrets:   manifest.Secrets,
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

func resolveAll(ctx context.Context, work report.Report, client puller, relocs []delivery.Relocation, platform, layout string) (map[string]string, error) {
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

func carryFiles(source fs.FS, mappings []delivery.Mapping) ([]delivery.File, error) {
	files := make([]delivery.File, 0, len(mappings))
	for _, mapping := range mappings {
		data, err := fs.ReadFile(source, mapping.Source)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", mapping.Source, err)
		}
		files = append(files, delivery.File{Path: strings.TrimPrefix(mapping.Target, "/"), Data: data, Mode: fs.FileMode(mapping.Mode)})
	}
	return files, nil
}

func imagesOf(digests map[string]string) []bundle.Image {
	images := make([]bundle.Image, 0, len(digests))
	for ref, digest := range digests {
		images = append(images, bundle.Image{Ref: ref, Digest: digest})
	}
	sort.Slice(images, func(a, b int) bool { return images[a].Ref < images[b].Ref })
	return images
}
