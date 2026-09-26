package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func putBlob(dir string, body []byte) (blob, error) {
	sum := sha256.Sum256(body)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	path := filepath.Join(dir, blobsDir, strings.TrimPrefix(digest, "sha256:"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return blob{}, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return blob{}, fmt.Errorf("write %s: %w", path, err)
	}
	return blob{Digest: digest, Size: int64(len(body))}, nil
}

// readBlob reads a blob and refuses content that does not hash to the name it sits under.
func readBlob(dir, digest string) ([]byte, error) {
	pin, err := v1.NewHash(digest)
	if err != nil {
		return nil, err
	}
	name := pin.Hex
	body, err := os.ReadFile(filepath.Join(dir, blobsDir, name))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", digest, err)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	if hash != name {
		return nil, fmt.Errorf("%s %w, hashes to sha256:%s", digest, ErrDigestMismatch, hash)
	}
	return body, nil
}
