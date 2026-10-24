package bundle

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
)

const (
	magic       = "VESSELv1"
	lengthSize  = 8
	trailerSize = len(magic) + lengthSize
)

var ErrNoPayload = errors.New("this file carries no bundle")

func Trailer(length int64) []byte {
	return binary.BigEndian.AppendUint64([]byte(magic), uint64(length))
}

func Payload(path string) (int64, int64, error) {
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
