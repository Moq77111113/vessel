package compose

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

const dotEnv = ".env"

var ErrLink = errors.New("a file compose uses is a link, vessel carries files")

func checkNoLink(dir fs.FS, name string) error {
	for prefix := name; prefix != "."; prefix = path.Dir(prefix) {
		info, err := fs.Lstat(dir, prefix)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", prefix, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s: %w", prefix, ErrLink)
		}
	}
	return nil
}

type Reader struct{}

func NewReader() *Reader { return &Reader{} }

func (r *Reader) Name() string { return "compose" }

func (r *Reader) Detect(dir fs.FS) bool {
	info, err := fs.Stat(dir, composeFile)
	return err == nil && info.Mode().IsRegular()
}

func (r *Reader) Read(dir fs.FS) (delivery.Manifest, error) {
	data, err := fs.ReadFile(dir, composeFile)
	if err != nil {
		return delivery.Manifest{}, fmt.Errorf("read %s: %w", composeFile, err)
	}
	file, err := readProject(data)
	if err != nil {
		return delivery.Manifest{}, err
	}
	relocs, err := imageRelocations(0, data, file)
	if err != nil {
		return delivery.Manifest{}, err
	}
	secrets, err := file.secretNames()
	if err != nil {
		return delivery.Manifest{}, err
	}
	if _, err := file.owners(); err != nil {
		return delivery.Manifest{}, err
	}
	beside, err := carryBeside(dir, file)
	if err != nil {
		return delivery.Manifest{}, err
	}
	return delivery.Manifest{
		Files:    append([]delivery.File{{Path: file.path(), Data: data}}, beside...),
		Relocs:   relocs,
		Services: []delivery.Service{{Name: file.Name, Path: file.path()}},
		Secrets:  secrets,
		Dirs:     []string{composePath},
	}, nil
}

func carryBeside(dir fs.FS, file project) ([]delivery.File, error) {
	sources, err := file.sources()
	if err != nil {
		return nil, err
	}
	if err := checkNoLink(dir, composeFile); err != nil {
		return nil, err
	}
	names := []string{dotEnv}
	for _, source := range sources {
		found, err := regularFiles(dir, source)
		if err != nil {
			return nil, err
		}
		names = append(names, found...)
	}
	sort.Strings(names)
	names = slices.Compact(names)
	var files []delivery.File
	for _, name := range names {
		if err := checkNoLink(dir, name); err != nil {
			return nil, err
		}
		data, err := fs.ReadFile(dir, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		files = append(files, delivery.File{Path: path.Join(file.dir(), name), Data: data})
	}
	return files, nil
}

func regularFiles(dir fs.FS, source string) ([]string, error) {
	if err := checkNoLink(dir, source); err != nil {
		return nil, err
	}
	var names []string
	err := fs.WalkDir(dir, source, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s: %w", name, ErrLink)
		}
		if entry.Type().IsRegular() {
			names = append(names, name)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	return names, nil
}
