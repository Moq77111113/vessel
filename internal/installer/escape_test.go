package installer

import (
	"archive/tar"
	"bytes"
	"encoding/binary"
	"os"
)

// packEscape writes a packed file whose payload tries to climb out of the target directory.
func packEscape(out string) error {
	var payload bytes.Buffer
	archive := tar.NewWriter(&payload)
	body := []byte("owned")
	header := &tar.Header{Name: "../escaped", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := archive.WriteHeader(header); err != nil {
		return err
	}
	if _, err := archive.Write(body); err != nil {
		return err
	}
	if err := archive.Close(); err != nil {
		return err
	}
	file := append(stub(), payload.Bytes()...)
	file = append(file, magic...)
	length := make([]byte, 8)
	binary.BigEndian.PutUint64(length, uint64(payload.Len()))
	return os.WriteFile(out, append(file, length...), 0o755)
}
