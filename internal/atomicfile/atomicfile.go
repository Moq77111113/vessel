// Package atomicfile puts a file down in one shot, so a power cut leaves no half-written file.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Write puts body at path through a temporary file, so a power cut leaves the old file or the
// new one, never a half-written one. The parent directory must already exist.
func Write(path string, body []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".vessel-*")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("flush %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Chmod(temp.Name(), mode); err != nil {
		return fmt.Errorf("set the mode of %s: %w", path, err)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("move into %s: %w", path, err)
	}
	return syncDir(dir)
}

// MkdirAll creates dir and every missing parent, flushing each new entry so a power cut keeps it.
func MkdirAll(dir string, mode fs.FileMode) error {
	_, err := os.Stat(dir)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	parent := filepath.Dir(dir)
	if err := MkdirAll(parent, mode); err != nil {
		return err
	}
	if err := os.Mkdir(dir, mode); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return syncDir(parent)
}

// Rename moves from to to and flushes the directory, so a power cut keeps the move.
func Rename(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("move into %s: %w", to, err)
	}
	return syncDir(filepath.Dir(to))
}

// syncDir forces the rename itself to disk: without it a power cut can lose the directory entry.
func syncDir(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	defer handle.Close()

	if err := handle.Sync(); err != nil {
		return fmt.Errorf("flush %s: %w", dir, err)
	}
	return nil
}
