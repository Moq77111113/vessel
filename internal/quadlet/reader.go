// Package quadlet reads a directory of systemd quadlet units.
package quadlet

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

const systemdPath = "etc/containers/systemd"

const imageKey = "Image="

// ErrNoUnit says the source directory holds no quadlet unit at all.
var ErrNoUnit = errors.New("no quadlet unit in the source directory")

var unitSuffixes = []string{".container", ".network", ".volume", ".pod", ".build", ".image", ".kube"}

// Reader turns a directory of quadlet units into a format-neutral manifest.
type Reader struct{}

// NewReader returns a reader for quadlet unit directories.
func NewReader() *Reader { return &Reader{} }

func (r *Reader) Name() string { return "quadlet" }

// Detect reports whether the directory holds at least one .container unit.
func (r *Reader) Detect(dir fs.FS) bool {
	entries, err := fs.ReadDir(dir, ".")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".container") {
			return true
		}
	}
	return false
}

// Read carries every unit file and marks the byte range of each Image= reference.
func (r *Reader) Read(dir fs.FS) (descriptor.Manifest, error) {
	names, err := unitNames(dir)
	if err != nil {
		return descriptor.Manifest{}, err
	}
	if len(names) == 0 {
		return descriptor.Manifest{}, ErrNoUnit
	}
	var manifest descriptor.Manifest
	for _, name := range names {
		data, err := fs.ReadFile(dir, name)
		if err != nil {
			return descriptor.Manifest{}, fmt.Errorf("read %s: %w", name, err)
		}
		index := len(manifest.Files)
		manifest.Files = append(manifest.Files, descriptor.File{
			Path: path.Join(systemdPath, name),
			Data: data,
		})
		relocs, err := imageRelocations(index, data)
		if err != nil {
			return descriptor.Manifest{}, fmt.Errorf("%s: %w", name, err)
		}
		manifest.Relocs = append(manifest.Relocs, relocs...)
	}
	return manifest, nil
}

func unitNames(dir fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(dir, ".")
	if err != nil {
		return nil, fmt.Errorf("read the source directory: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		for _, suffix := range unitSuffixes {
			if strings.HasSuffix(entry.Name(), suffix) {
				names = append(names, entry.Name())
				break
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

func imageRelocations(index int, data []byte) ([]descriptor.Relocation, error) {
	var relocs []descriptor.Relocation
	offset := 0
	for line := range bytes.SplitAfterSeq(data, []byte("\n")) {
		start, value := imageValue(line)
		if start < 0 {
			offset += len(line)
			continue
		}
		ref, err := descriptor.ParseRef(string(value))
		if err != nil {
			return nil, err
		}
		relocs = append(relocs, descriptor.Relocation{
			File:   index,
			Offset: offset + start,
			Length: len(value),
			Ref:    ref,
		})
		offset += len(line)
	}
	return relocs, nil
}

func imageValue(line []byte) (int, []byte) {
	trimmed := bytes.TrimLeft(line, " \t")
	if len(trimmed) == 0 || trimmed[0] == '#' || trimmed[0] == ';' {
		return -1, nil
	}
	if !bytes.HasPrefix(trimmed, []byte(imageKey)) {
		return -1, nil
	}
	lead := len(line) - len(trimmed)
	value := bytes.TrimRight(trimmed[len(imageKey):], " \t\r\n")
	if len(value) == 0 {
		return -1, nil
	}
	return lead + len(imageKey), value
}
