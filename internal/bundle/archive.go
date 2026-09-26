package bundle

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

func packFiles(files []descriptor.File) ([]byte, error) {
	byPath := append([]descriptor.File(nil), files...)
	sort.Slice(byPath, func(a, b int) bool { return byPath[a].Path < byPath[b].Path })

	var out strings.Builder
	archive := tar.NewWriter(&out)
	for _, file := range byPath {
		// Mode 0 says the descriptor declared none; a bundle built before modes says 0644 for every file.
		header := &tar.Header{
			Name:     file.Path,
			Mode:     int64(file.Mode),
			Size:     int64(len(file.Data)),
			Typeflag: tar.TypeReg,
		}
		if err := archive.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("write the header of %s: %w", file.Path, err)
		}
		if _, err := archive.Write(file.Data); err != nil {
			return nil, fmt.Errorf("write %s: %w", file.Path, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, fmt.Errorf("close the file archive: %w", err)
	}
	return []byte(out.String()), nil
}

func unpackFiles(body []byte) ([]descriptor.File, error) {
	var files []descriptor.File
	archive := tar.NewReader(strings.NewReader(string(body)))
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read the file archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if err := safePath(header.Name); err != nil {
			return nil, err
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", header.Name, err)
		}
		files = append(files, descriptor.File{Path: header.Name, Data: data, Mode: fs.FileMode(header.Mode).Perm()})
	}
}

func safePath(name string) error {
	clean := filepath.ToSlash(filepath.Clean(name))
	if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("bundle path %q %w", name, ErrPathEscapes)
	}
	return nil
}
