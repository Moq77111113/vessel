package site

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Moq77111113/vessel/internal/bundle"
)

var ErrPathEscapes = errors.New("leaves the target directory")

func CarriesABundle(path string) bool {
	_, _, err := bundle.Payload(path)
	return err == nil
}

func Unpack(path, dir string) error {
	offset, length, err := bundle.Payload(path)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	archive := tar.NewReader(io.NewSectionReader(file, offset, length))
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read the bundle out of %s: %w", path, err)
		}
		target, err := resolve(dir, header.Name)
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
		}
		body, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("create %s: %w", target, err)
		}
		if _, err := io.Copy(body, archive); err != nil {
			body.Close()
			return fmt.Errorf("write %s: %w", target, err)
		}
		if err := body.Close(); err != nil {
			return fmt.Errorf("close %s: %w", target, err)
		}
	}
}

func resolve(dir, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q %w", name, ErrPathEscapes)
	}
	return filepath.Join(dir, clean), nil
}
