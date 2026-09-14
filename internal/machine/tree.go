package machine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// ErrPathEscapes says a bundle file asked for a path outside the target root.
var ErrPathEscapes = errors.New("leaves the target root")

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
	if Same(path, DigestOf(file.Data)) {
		return false, nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("create %s: %w", dir, err)
	}
	if err := replace(path, file.Data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// Remove deletes the file at name under the root and reports whether it was there to delete.
func (t *Tree) Remove(name string) (bool, error) {
	path, err := t.resolve(name)
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("remove %s: %w", path, err)
	}
	return true, nil
}

func (t *Tree) resolve(name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q %w", name, ErrPathEscapes)
	}
	return filepath.Join(t.root, clean), nil
}

// Same reports whether the file at path still hashes to digest.
func Same(path, digest string) bool {
	body, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return DigestOf(body) == digest
}

// DigestOf is the sha256 of data, in the form a bundle records.
func DigestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
