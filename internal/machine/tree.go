package machine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Moq77111113/vessel/internal/atomicfile"
	"github.com/Moq77111113/vessel/internal/descriptor"
)

// Errors a tree returns before it touches a path.
var (
	ErrPathEscapes  = errors.New("leaves the target root")
	ErrUnsafeParent = errors.New("sits under a directory another user can change")
)

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
	if err := t.checkParents(path); err != nil {
		return false, err
	}
	mode := modeFor(path, file.Mode)
	if Same(path, DigestOf(file.Data)) && permOf(path) == mode {
		return false, nil
	}
	dir := filepath.Dir(path)
	if err := atomicfile.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if err := atomicfile.Write(path, file.Data, mode); err != nil {
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
	if err := t.checkParents(path); err != nil {
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

// modeFor is the declared mode, else the mode of the file in place, else readable by all.
func modeFor(path string, declared fs.FileMode) fs.FileMode {
	if declared != 0 {
		return declared
	}
	if perm := permOf(path); perm != 0 {
		return perm
	}
	return 0o644
}

// permOf is the permission bits of the file at path, or 0 when nothing is there.
func permOf(path string) fs.FileMode {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Mode().Perm()
}

// Holds reports whether anything sits at name under the root.
func (t *Tree) Holds(name string) (bool, error) {
	path, err := t.resolve(name)
	if err != nil {
		return false, err
	}
	_, err = os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return true, nil
}

// Differs reports whether the file at name is there and no longer hashes to digest.
func (t *Tree) Differs(name, digest string) (bool, error) {
	path, err := t.resolve(name)
	if err != nil {
		return false, err
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	return DigestOf(body) != digest, nil
}

func (t *Tree) resolve(name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q %w", name, ErrPathEscapes)
	}
	return filepath.Join(t.root, clean), nil
}

// checkParents refuses a path when a directory leading to it, or one a link leads to, is not this process's alone.
func (t *Tree) checkParents(path string) error {
	for dir := filepath.Dir(path); within(t.root, dir); dir = filepath.Dir(dir) {
		real, err := filepath.EvalSymlinks(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("resolve %s: %w", dir, err)
		}
		if err := t.checkChain(real); err != nil {
			return fmt.Errorf("%s %w", path, err)
		}
	}
	return nil
}

// checkChain refuses dir, or a directory above it up to the tree root or /, that is not ours alone.
func (t *Tree) checkChain(dir string) error {
	top := string(filepath.Separator)
	if within(t.root, dir) {
		top = t.root
	}
	for ; ; dir = filepath.Dir(dir) {
		if err := ours(dir); err != nil {
			return err
		}
		if dir == top || dir == filepath.Dir(dir) {
			return nil
		}
	}
}

// ours refuses a directory that another user owns, or that its group or anyone may write.
func ours(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	owner := info.Sys().(*syscall.Stat_t).Uid
	if int(owner) != os.Getuid() || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s: %w", dir, ErrUnsafeParent)
	}
	return nil
}

// within reports whether path is root or sits under it.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
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
