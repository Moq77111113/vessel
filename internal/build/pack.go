package build

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
)

var ErrNotAFile = errors.New("is not a regular file")

type packer struct {
	work report.Report
}

func (p packer) pack(stub io.Reader, dir string, out io.Writer) error {
	if _, err := io.Copy(out, stub); err != nil {
		return fmt.Errorf("copy the stub: %w", err)
	}
	counter := &counting{writer: out}
	if err := writeTar(counter, dir, p.work); err != nil {
		return err
	}
	if _, err := out.Write(bundle.Trailer(counter.bytes)); err != nil {
		return fmt.Errorf("write the trailer: %w", err)
	}
	return nil
}

func writeTar(out io.Writer, dir string, work report.Report) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("open %s: %w", dir, err)
	}
	defer root.Close()
	archive := tar.NewWriter(out)
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		work.Line("Writing", name)
		if err := writePart(archive, root, name); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("pack %s: %w", dir, err)
	}
	return archive.Close()
}

func writePart(archive *tar.Writer, root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrNotAFile
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	header := &tar.Header{Name: name, Mode: 0o644, Size: info.Size(), Typeflag: tar.TypeReg}
	if err := archive.WriteHeader(header); err != nil {
		return err
	}
	_, err = io.Copy(archive, file)
	return err
}

type counting struct {
	writer io.Writer
	bytes  int64
}

func (c *counting) Write(data []byte) (int, error) {
	n, err := c.writer.Write(data)
	c.bytes += int64(n)
	return n, err
}
