package oci

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Moq77111113/vessel/internal/bundle"
)

const RefNameAnnotation = "org.opencontainers.image.ref.name"

var (
	ErrBlobDigest  = errors.New("blob content does not match its digest")
	ErrBlobMissing = errors.New("blob missing from the layout")
)

type blobRef struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type indexFile struct {
	SchemaVersion int        `json:"schemaVersion"`
	MediaType     string     `json:"mediaType,omitempty"`
	ArtifactType  string     `json:"artifactType,omitempty"`
	Manifests     []manifest `json:"manifests"`
}

type manifest struct {
	MediaType    string            `json:"mediaType"`
	Digest       string            `json:"digest"`
	Size         int64             `json:"size"`
	Annotations  map[string]string `json:"annotations,omitempty"`
	ArtifactType string            `json:"artifactType,omitempty"`
}

type imageManifest struct {
	Config blobRef   `json:"config"`
	Layers []blobRef `json:"layers"`
}

type Images struct {
	dir   string
	index indexFile
}

func Open(dir, root string) (*Images, error) {
	layout := &Images{dir: dir}
	body, err := layout.blob(root)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &layout.index); err != nil {
		return nil, fmt.Errorf("decode the index %s: %w", root, err)
	}
	return layout, nil
}

func (l *Images) images() []manifest {
	var images []manifest
	for _, entry := range l.index.Manifests {
		switch entry.ArtifactType {
		case bundle.ArtifactType, bundle.FilesType:
			continue
		}
		if entry.Annotations[RefNameAnnotation] != "" {
			images = append(images, entry)
		}
	}
	return images
}

func (l *Images) Names() ([]string, error) {
	images := l.images()
	names := make([]string, 0, len(images))
	for _, entry := range images {
		names = append(names, entry.Annotations[RefNameAnnotation])
	}
	return names, nil
}

func (l *Images) Archive(i int, out io.Writer) error {
	entry := l.images()[i]
	body, err := l.blob(entry.Digest)
	if err != nil {
		return err
	}
	var image imageManifest
	if err := json.Unmarshal(body, &image); err != nil {
		return fmt.Errorf("decode manifest %s: %w", entry.Digest, err)
	}

	digests := []string{entry.Digest, image.Config.Digest}
	for _, layer := range image.Layers {
		digests = append(digests, layer.Digest)
	}

	archive := tar.NewWriter(out)
	for _, dir := range []string{"blobs", "blobs/sha256"} {
		if err := archive.WriteHeader(&tar.Header{Name: dir + "/", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
			return fmt.Errorf("write the header of %s: %w", dir, err)
		}
	}
	for _, digest := range digests {
		if err := l.copyBlob(archive, digest); err != nil {
			return err
		}
	}
	only := indexFile{SchemaVersion: 2, MediaType: l.index.MediaType, Manifests: []manifest{entry}}
	index, err := json.Marshal(only)
	if err != nil {
		return fmt.Errorf("encode the image index: %w", err)
	}
	if err := writeEntry(archive, "index.json", index); err != nil {
		return err
	}
	layout, err := os.ReadFile(filepath.Join(l.dir, "oci-layout"))
	if err != nil {
		return fmt.Errorf("read oci-layout: %w", err)
	}
	if err := writeEntry(archive, "oci-layout", layout); err != nil {
		return err
	}
	return archive.Close()
}

func (l *Images) copyBlob(archive *tar.Writer, digest string) error {
	name := strings.TrimPrefix(digest, "sha256:")
	file, err := os.Open(filepath.Join(l.dir, "blobs", "sha256", name))
	if err != nil {
		return fmt.Errorf("%s: %w", digest, ErrBlobMissing)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", digest, err)
	}
	header := &tar.Header{Name: "blobs/sha256/" + name, Mode: 0o644, Size: info.Size(), Typeflag: tar.TypeReg}
	if err := archive.WriteHeader(header); err != nil {
		return fmt.Errorf("write the header of %s: %w", digest, err)
	}
	hash := sha256.New()
	if _, err := io.Copy(archive, io.TeeReader(file, hash)); err != nil {
		return fmt.Errorf("write %s: %w", digest, err)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != name {
		return fmt.Errorf("%s: %w, hashes to sha256:%s", digest, ErrBlobDigest, actual)
	}
	return nil
}

func (l *Images) blob(digest string) ([]byte, error) {
	name := strings.TrimPrefix(digest, "sha256:")
	data, err := os.ReadFile(filepath.Join(l.dir, "blobs", "sha256", name))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", digest, ErrBlobMissing)
	}
	sum := sha256.Sum256(data)
	if actual := hex.EncodeToString(sum[:]); actual != name {
		return nil, fmt.Errorf("%s: %w, hashes to sha256:%s", digest, ErrBlobDigest, actual)
	}
	return data, nil
}

func writeEntry(archive *tar.Writer, name string, data []byte) error {
	header := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}
	if err := archive.WriteHeader(header); err != nil {
		return fmt.Errorf("write the header of %s: %w", name, err)
	}
	if _, err := archive.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func (l *Images) Count() int { return len(l.images()) }
