// Package bundle reads and writes the OCI artifact vessel produces.
//
// A bundle is a plain OCI layout, so any registry or OCI tool can carry it.
//
// The root is an image index, not a manifest, because an index takes its children with it
// through a registry: copy the bundle and the images follow. That index references every
// image manifest plus one manifest holding the config and the descriptor files. Its digest
// is the root of everything, and it is what a signature covers.
package bundle

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
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/descriptor"
)

const (
	// ArtifactType marks the index that is the bundle root.
	ArtifactType = "application/vnd.vessel.bundle.v1"
	// FilesType marks the manifest holding the config and the descriptor files.
	FilesType    = "application/vnd.vessel.files.manifest.v1"
	configType   = "application/vnd.vessel.bundle.config.v1+json"
	filesType    = "application/vnd.vessel.files.v1.tar"
	manifestType = "application/vnd.oci.image.manifest.v1+json"
	indexType    = "application/vnd.oci.image.index.v1+json"

	// refNameAnnotation is how an OCI layout names a manifest, so a tool can address it.
	refNameAnnotation = "org.opencontainers.image.ref.name"

	indexName  = "index.json"
	layoutName = "oci-layout"
	blobsDir   = "blobs/sha256"
	// SignatureName is the detached signature over the bundle manifest.
	SignatureName = "vessel.sig"
)

// Errors Open returns when a bundle no longer matches what it says it is.
var (
	ErrDigestMismatch = errors.New("does not match the digest that pins it")
	ErrPathEscapes    = errors.New("leaves the target root")
	ErrNotABundle     = errors.New("is not a vessel bundle")
)

// Image is one image reference and the digest it resolved to.
type Image struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
}

// Config is the bundle manifest's config blob: what this bundle is, every image in it, and the
// variables and actions its install side needs.
type Config struct {
	Name      string              `json:"name"`
	Version   string              `json:"version"`
	Reader    string              `json:"reader"`
	Platform  string              `json:"platform"`
	Time      string              `json:"time"`
	Images    []Image             `json:"images"`
	Variables []delivery.Variable `json:"variables,omitempty"`
	Actions   []string            `json:"actions,omitempty"`
}

// Writing is everything a link run hands to Write.
type Writing struct {
	Config Config
	Layout string
	Files  []descriptor.File
}

// Bundle is an opened artifact whose parts match the digests that pin them.
type Bundle struct {
	Config    Config
	Files     []descriptor.File
	LayoutDir string
	// Root is the bundle manifest digest, the value a signature covers.
	Root string
}

type blob struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	ArtifactType  string `json:"artifactType,omitempty"`
	Config        blob   `json:"config"`
	Layers        []blob `json:"layers"`
}

type entry struct {
	MediaType    string            `json:"mediaType"`
	ArtifactType string            `json:"artifactType,omitempty"`
	Digest       string            `json:"digest"`
	Size         int64             `json:"size"`
	Annotations  map[string]string `json:"annotations,omitempty"`
}

type index struct {
	SchemaVersion int     `json:"schemaVersion"`
	MediaType     string  `json:"mediaType"`
	ArtifactType  string  `json:"artifactType,omitempty"`
	Manifests     []entry `json:"manifests"`
}

// Write turns the image layout into a bundle by adding the files and the bundle index.
func Write(dir string, w Writing) error {
	if err := copyTree(w.Layout, dir); err != nil {
		return err
	}
	carried, err := readIndex(dir)
	if err != nil {
		return err
	}
	files, err := filesManifest(dir, w)
	if err != nil {
		return err
	}
	root, err := putBlob(dir, mustMarshal(index{
		SchemaVersion: 2,
		MediaType:     indexType,
		ArtifactType:  ArtifactType,
		Manifests:     append(carried.Manifests, files),
	}))
	if err != nil {
		return err
	}
	return writeIndex(dir, index{
		SchemaVersion: 2,
		MediaType:     indexType,
		Manifests: []entry{{
			MediaType:    indexType,
			ArtifactType: ArtifactType,
			Digest:       root.Digest,
			Size:         root.Size,
			Annotations:  map[string]string{refNameAnnotation: w.Config.Name + ":" + w.Config.Version},
		}},
	})
}

// filesManifest writes the config and the descriptor files, and returns the manifest over them.
func filesManifest(dir string, w Writing) (entry, error) {
	archive, err := packFiles(w.Files)
	if err != nil {
		return entry{}, err
	}
	layer, err := putBlob(dir, archive)
	if err != nil {
		return entry{}, err
	}
	config, err := putBlob(dir, mustMarshal(w.Config))
	if err != nil {
		return entry{}, err
	}
	body, err := putBlob(dir, mustMarshal(manifest{
		SchemaVersion: 2,
		MediaType:     manifestType,
		ArtifactType:  FilesType,
		Config:        blob{MediaType: configType, Digest: config.Digest, Size: config.Size},
		Layers:        []blob{{MediaType: filesType, Digest: layer.Digest, Size: layer.Size}},
	}))
	if err != nil {
		return entry{}, err
	}
	return entry{
		MediaType:    manifestType,
		ArtifactType: FilesType,
		Digest:       body.Digest,
		Size:         body.Size,
	}, nil
}

// Open reads a bundle and refuses any part whose digest left its manifest behind.
func Open(dir string) (*Bundle, error) {
	root, err := bundleEntry(dir)
	if err != nil {
		return nil, err
	}
	body, err := readBlob(dir, root.Digest)
	if err != nil {
		return nil, err
	}
	var bundleIndex index
	if err := json.Unmarshal(body, &bundleIndex); err != nil {
		return nil, fmt.Errorf("decode the bundle index: %w", err)
	}
	files, err := filesEntry(bundleIndex)
	if err != nil {
		return nil, err
	}
	body, err = readBlob(dir, files.Digest)
	if err != nil {
		return nil, err
	}
	var carried manifest
	if err := json.Unmarshal(body, &carried); err != nil {
		return nil, fmt.Errorf("decode the files manifest: %w", err)
	}
	config, err := readBlob(dir, carried.Config.Digest)
	if err != nil {
		return nil, err
	}
	var decoded Config
	if err := json.Unmarshal(config, &decoded); err != nil {
		return nil, fmt.Errorf("decode the bundle config: %w", err)
	}
	if len(carried.Layers) != 1 {
		return nil, fmt.Errorf("the bundle manifest carries %d layers, want 1", len(carried.Layers))
	}
	archive, err := readBlob(dir, carried.Layers[0].Digest)
	if err != nil {
		return nil, err
	}
	unpacked, err := unpackFiles(archive)
	if err != nil {
		return nil, err
	}
	return &Bundle{Config: decoded, Files: unpacked, LayoutDir: dir, Root: root.Digest}, nil
}

// IsBundle reports whether dir is an OCI layout holding a vessel bundle manifest.
func IsBundle(dir string) bool {
	_, err := bundleEntry(dir)
	return err == nil
}

// RootBytes returns the bundle manifest digest, the value a signature covers.
func RootBytes(dir string) ([]byte, error) {
	root, err := bundleEntry(dir)
	if err != nil {
		return nil, err
	}
	return []byte(root.Digest), nil
}

// bundleEntry finds the bundle root by reading each candidate index, not by trusting the
// entry that points at it: a registry round trip drops artifactType from the entry while
// the blob it names keeps it.
func bundleEntry(dir string) (entry, error) {
	var carried index
	if err := readJSON(filepath.Join(dir, indexName), &carried); err != nil {
		return entry{}, fmt.Errorf("%s %w", dir, ErrNotABundle)
	}
	var corrupted error
	for _, candidate := range carried.Manifests {
		if candidate.MediaType != indexType {
			continue
		}
		body, err := readBlob(dir, candidate.Digest)
		if errors.Is(err, ErrDigestMismatch) {
			corrupted = err
			continue
		}
		if err != nil {
			continue
		}
		var inner index
		if err := json.Unmarshal(body, &inner); err != nil {
			continue
		}
		if inner.ArtifactType == ArtifactType {
			return candidate, nil
		}
	}
	// A blob that does not hash to its name is a damaged bundle, not a foreign directory.
	if corrupted != nil {
		return entry{}, corrupted
	}
	return entry{}, fmt.Errorf("%s %w", dir, ErrNotABundle)
}

func readIndex(dir string) (index, error) {
	var carried index
	if err := readJSON(filepath.Join(dir, indexName), &carried); err != nil {
		return index{}, err
	}
	return carried, nil
}

func writeIndex(dir string, carried index) error {
	path := filepath.Join(dir, indexName)
	if err := os.WriteFile(path, mustMarshal(carried), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func filesEntry(bundleIndex index) (entry, error) {
	for _, candidate := range bundleIndex.Manifests {
		if candidate.ArtifactType == FilesType {
			return candidate, nil
		}
	}
	return entry{}, fmt.Errorf("the bundle index holds no %s manifest", FilesType)
}

// mustMarshal encodes a value vessel just built, so it cannot carry an unencodable field.
func mustMarshal(value any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

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
	name := strings.TrimPrefix(digest, "sha256:")
	body, err := os.ReadFile(filepath.Join(dir, blobsDir, name))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", digest, err)
	}
	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != name {
		return nil, fmt.Errorf("%s %w, hashes to sha256:%s", digest, ErrDigestMismatch, got)
	}
	return body, nil
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk the image layout: %w", err)
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return fmt.Errorf("walk the image layout: %w", err)
		}
		target := filepath.Join(to, rel)
		if entry.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}

func packFiles(files []descriptor.File) ([]byte, error) {
	ordered := append([]descriptor.File(nil), files...)
	sort.Slice(ordered, func(a, b int) bool { return ordered[a].Path < ordered[b].Path })

	var out strings.Builder
	archive := tar.NewWriter(&out)
	for _, file := range ordered {
		header := &tar.Header{
			Name:     file.Path,
			Mode:     0o644,
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
		files = append(files, descriptor.File{Path: header.Name, Data: data})
	}
}

func safePath(name string) error {
	clean := filepath.ToSlash(filepath.Clean(name))
	if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("bundle path %q %w", name, ErrPathEscapes)
	}
	return nil
}

func readJSON(path string, value any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(body, value); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}
