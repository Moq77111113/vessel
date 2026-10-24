package build

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/Moq77111113/vessel/internal/build/registry"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

const defaultPlatform = "linux/amd64"

var (
	ErrSecretNotRead           = errors.New("no service reads this secret")
	ErrTargetInDriverDirectory = errors.New("a file target lands in a directory the driver fills")
)

type Link struct {
	Drivers []driver.Driver
}

type LinkRequest struct {
	Source           string
	Name             string
	Version          string
	Platform         string
	InsecureUnsigned bool
	Dir              string
	Work             report.Report
}

type LinkResult struct {
	Config bundle.Config
}

func (l Link) Run(ctx context.Context, req LinkRequest) (LinkResult, error) {
	source, err := os.OpenRoot(req.Source)
	if err != nil {
		return LinkResult{}, fmt.Errorf("open %s: %w", req.Source, err)
	}
	defer source.Close()
	definition, err := delivery.Read(source.FS())
	if err != nil && !errors.Is(err, delivery.ErrNoDelivery) {
		return LinkResult{}, err
	}
	config, err := req.config(definition)
	if err != nil {
		return LinkResult{}, err
	}
	kind, err := l.pick(source.FS(), definition)
	if err != nil {
		return LinkResult{}, err
	}
	manifest, err := carry(kind, source.FS(), definition)
	if err != nil {
		return LinkResult{}, err
	}
	config.Machine = kind.Name()
	config.Services = manifest.Services
	config.Secrets = manifest.Secrets
	config, err = req.write(ctx, manifest, config)
	if err != nil {
		return LinkResult{}, err
	}
	return LinkResult{Config: config}, nil
}

func (r LinkRequest) config(definition delivery.Delivery) (bundle.Config, error) {
	config := bundle.Config{
		Name:      definition.Name,
		Version:   definition.Version,
		Platform:  defaultPlatform,
		Variables: definition.Variables,
		Actions:   definition.Actions,
		Insecure:  r.InsecureUnsigned,
	}
	if r.Name != "" {
		config.Name = r.Name
	}
	if r.Version != "" {
		config.Version = r.Version
	}
	if r.Platform != "" {
		config.Platform = r.Platform
	}
	return config, delivery.CheckName(config.Name)
}

func (l Link) pick(source fs.FS, definition delivery.Delivery) (driver.Driver, error) {
	dir, err := units(source, definition)
	if err != nil {
		return nil, err
	}
	return driver.Pick(l.Drivers, dir)
}

func units(source fs.FS, definition delivery.Delivery) (fs.FS, error) {
	if definition.Source == "" {
		return source, nil
	}
	return fs.Sub(source, definition.Source)
}

func carry(kind driver.Driver, source fs.FS, definition delivery.Delivery) (delivery.Manifest, error) {
	dir, err := units(source, definition)
	if err != nil {
		return delivery.Manifest{}, err
	}
	manifest, err := kind.Read(dir)
	if err != nil {
		return delivery.Manifest{}, err
	}
	plain, err := carryFiles(source, definition.Files)
	if err != nil {
		return delivery.Manifest{}, err
	}
	if err := delivery.CheckCollisions(manifest.Files, plain); err != nil {
		return delivery.Manifest{}, err
	}
	if err := checkOutsideTheDriverDirectories(manifest.Dirs, plain); err != nil {
		return delivery.Manifest{}, err
	}
	if err := checkEverySecretIsRead(manifest.Secrets, definition.Variables); err != nil {
		return delivery.Manifest{}, err
	}
	manifest.Files = append(manifest.Files, plain...)
	return manifest, delivery.CheckMarkers(manifest.Files, definition.Variables)
}

func (r LinkRequest) write(ctx context.Context, manifest delivery.Manifest, config bundle.Config) (bundle.Config, error) {
	layout, err := os.MkdirTemp("", "vessel-layout-*")
	if err != nil {
		return bundle.Config{}, fmt.Errorf("create a working directory: %w", err)
	}
	defer os.RemoveAll(layout)
	digests, err := pull{client: registry.New(), work: r.Work, platform: config.Platform, layout: layout}.resolve(ctx, manifest.Relocs)
	if err != nil {
		return bundle.Config{}, err
	}
	files, err := bundle.Patch(manifest, digests)
	if err != nil {
		return bundle.Config{}, err
	}
	config.Images = images(digests)
	return config, bundle.Write(r.Dir, bundle.Contents{Layout: layout, Files: files, Config: config})
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

func checkOutsideTheDriverDirectories(dirs []string, files []delivery.File) error {
	for _, file := range files {
		for _, dir := range dirs {
			if strings.HasPrefix(path.Clean(file.Path), dir+"/") {
				return fmt.Errorf("%s: %w", file.Path, ErrTargetInDriverDirectory)
			}
		}
	}
	return nil
}

func checkEverySecretIsRead(read []string, variables []delivery.Variable) error {
	for _, name := range delivery.SecretNames(variables) {
		if !slices.Contains(read, name) {
			return fmt.Errorf("%s: %w", name, ErrSecretNotRead)
		}
	}
	return nil
}
