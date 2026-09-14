package machine

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// replace puts body at path through a temporary file, so a power cut leaves the old file or the
// new one, never a half-written one. The parent directory must already exist.
func replace(path string, body []byte, mode fs.FileMode) error {
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
