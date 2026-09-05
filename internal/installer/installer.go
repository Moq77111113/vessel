// Package installer turns a bundle into one executable that installs itself.
package installer

import (
	"archive/tar"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	// magic marks the trailer at the very end of a packed file.
	magic = "VESSELv1"
	// lengthSize is the payload length, big endian, that follows the magic.
	lengthSize = 8
	// trailerSize is read backwards from the end of the file.
	trailerSize = len(magic) + lengthSize
)

// Errors a packed file returns when it does not hold what was asked of it.
var (
	ErrNoPayload   = errors.New("this file carries no bundle")
	ErrPathEscapes = errors.New("leaves the target directory")
)

// Pack writes stub followed by the bundle and a trailer, as one executable file.
func Pack(stub io.Reader, bundle, out string) error {
	file, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create %s: %w", out, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, stub); err != nil {
		return fmt.Errorf("copy the stub into %s: %w", out, err)
	}
	counter := &counting{writer: file}
	if err := writeTar(counter, bundle); err != nil {
		return err
	}
	if _, err := io.WriteString(file, magic); err != nil {
		return fmt.Errorf("write the trailer of %s: %w", out, err)
	}
	length := make([]byte, lengthSize)
	binary.BigEndian.PutUint64(length, uint64(counter.written))
	if _, err := file.Write(length); err != nil {
		return fmt.Errorf("write the trailer of %s: %w", out, err)
	}
	return nil
}

// CarriesABundle reports whether a file was produced by Pack.
func CarriesABundle(path string) bool {
	_, _, err := payload(path)
	return err == nil
}

// Unpack writes the bundle a packed file carries into dir.
func Unpack(path, dir string) error {
	offset, length, err := payload(path)
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

func payload(path string) (int64, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return 0, 0, fmt.Errorf("read %s: %w", path, err)
	}
	if info.Size() < int64(trailerSize) {
		return 0, 0, ErrNoPayload
	}
	trailer := make([]byte, trailerSize)
	if _, err := file.ReadAt(trailer, info.Size()-int64(trailerSize)); err != nil {
		return 0, 0, fmt.Errorf("read the trailer of %s: %w", path, err)
	}
	if string(trailer[:len(magic)]) != magic {
		return 0, 0, ErrNoPayload
	}
	length := int64(binary.BigEndian.Uint64(trailer[len(magic):]))
	offset := info.Size() - int64(trailerSize) - length
	if length <= 0 || offset < 0 {
		return 0, 0, ErrNoPayload
	}
	return offset, length, nil
}

func resolve(dir, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q %w", name, ErrPathEscapes)
	}
	return filepath.Join(dir, clean), nil
}

func writeTar(out io.Writer, bundle string) error {
	archive := tar.NewWriter(out)
	err := filepath.WalkDir(bundle, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name, err := filepath.Rel(bundle, path)
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		name = filepath.ToSlash(name)
		if entry.IsDir() {
			return archive.WriteHeader(&tar.Header{Name: name + "/", Mode: 0o755, Typeflag: tar.TypeDir})
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		_, err = archive.Write(data)
		return err
	})
	if err != nil {
		return fmt.Errorf("pack %s: %w", bundle, err)
	}
	return archive.Close()
}

// counting counts what goes through it, so the trailer can say how long the payload is.
type counting struct {
	writer  io.Writer
	written int64
}

func (c *counting) Write(data []byte) (int, error) {
	n, err := c.writer.Write(data)
	c.written += int64(n)
	return n, err
}
