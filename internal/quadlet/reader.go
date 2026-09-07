// Package quadlet reads a directory of systemd quadlet units.
package quadlet

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

const systemdPath = "etc/containers/systemd"

// siblingPath is where a plain systemd unit goes: quadlet does not generate those, systemd reads
// them directly.
const siblingPath = "etc/systemd/system"

const imageKey = "Image"

// ErrNoUnit says the source directory holds no quadlet unit at all.
var ErrNoUnit = errors.New("no quadlet unit in the source directory")

var unitSuffixes = []string{".container", ".network", ".volume", ".pod", ".build", ".image", ".kube"}

var siblingSuffixes = []string{".timer", ".socket", ".path"}

// carriedSuffixes is every unit file kind vessel takes out of a source directory.
var carriedSuffixes = slices.Concat(unitSuffixes, siblingSuffixes)

// pathFor returns where a unit file goes under the target root.
func pathFor(name string) string {
	for _, suffix := range siblingSuffixes {
		if strings.HasSuffix(name, suffix) {
			return path.Join(siblingPath, name)
		}
	}
	return path.Join(systemdPath, name)
}

// Owns reports whether a path under the target root lands in a directory this reader fills.
func (r *Reader) Owns(name string) bool {
	return under(name, systemdPath) || under(name, siblingPath)
}

// under reports whether a path lands inside a directory, at any depth.
func under(name, dir string) bool {
	return strings.HasPrefix(path.Clean(name), dir+"/")
}

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
			Path: pathFor(name),
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
		if carries(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func carries(name string) bool {
	for _, suffix := range carriedSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
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
	rest, ok := cutKey(trimmed, imageKey)
	if !ok {
		return -1, nil
	}
	value := bytes.TrimRight(rest, " \t\r\n")
	if len(value) == 0 {
		return -1, nil
	}
	return len(line) - len(rest), value
}
