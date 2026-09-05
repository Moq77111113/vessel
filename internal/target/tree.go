package target

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// ErrPathEscapes says a bundle file asked for a path outside the target root.
var ErrPathEscapes = errors.New("leaves the target root")

// systemdPath is where quadlet units live under the target root.
const systemdPath = "etc/containers/systemd"

// Tree is the file tree of the target machine, rooted where a bundle is written.
type Tree struct {
	root string
}

// NewTree returns the file tree rooted at dir.
func NewTree(dir string) *Tree { return &Tree{root: dir} }

// Write puts the file in place and reports whether the content on disk differed.
func (t *Tree) Write(file descriptor.File) (bool, error) {
	path, err := t.resolve(file.Path)
	if err != nil {
		return false, err
	}
	if same(path, file.Data) {
		return false, nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", dir, err)
	}
	temp, err := os.CreateTemp(dir, ".vessel-*")
	if err != nil {
		return false, fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(file.Data); err != nil {
		temp.Close()
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return false, fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Chmod(temp.Name(), 0o644); err != nil {
		return false, fmt.Errorf("set the mode of %s: %w", path, err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return false, fmt.Errorf("move into %s: %w", path, err)
	}
	return true, nil
}

func (t *Tree) resolve(name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q %w", name, ErrPathEscapes)
	}
	return filepath.Join(t.root, clean), nil
}

func same(path string, data []byte) bool {
	current, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return bytes.Equal(current, data)
}
