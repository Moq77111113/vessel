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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
)

// Errors Open returns when a bundle no longer matches what it says it is.
var (
	ErrDigestMismatch   = errors.New("does not match the digest that pins it")
	ErrPathEscapes      = errors.New("leaves the target root")
	ErrNotABundle       = errors.New("is not a vessel bundle")
	ErrBundleShape      = errors.New("is not shaped like a vessel bundle")
	ErrEvidenceName     = errors.New("an evidence name is not a plain file name")
	ErrEvidenceTwice    = errors.New("the bundle already carries other evidence of that name")
	ErrEvidenceSameName = errors.New("two evidence files share one name")
)

// Image is one image reference and the digest it resolved to.
type Image struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
}

// Config is the bundle manifest's config blob: what this bundle is, every image in it, and the
// variables and actions its install side needs.
type Config struct {
	Name      string                `json:"name"`
	Version   string                `json:"version"`
	Machine   string                `json:"machine"`
	Platform  string                `json:"platform"`
	Images    []Image               `json:"images"`
	Variables []descriptor.Variable `json:"variables,omitempty"`
	Actions   []string              `json:"actions,omitempty"`
	Insecure  bool                  `json:"insecure,omitempty"`
}

// Contents is everything a link run hands to Write.
type Contents struct {
	Config Config
	Layout string
	Files  []descriptor.File
}

// Bundle is an artifact whose parts match the digests that pin them.
type Bundle struct {
	Config    Config
	Files     []descriptor.File
	Evidence  []Evidence
	LayoutDir string
	// Root is the bundle manifest digest, the value a signature covers.
	Root string
}

// Write turns the image layout into a bundle by adding the files and the bundle index.
func Write(dir string, contents Contents) error {
	if err := copyTree(contents.Layout, dir); err != nil {
		return err
	}
	indexFile, err := readIndex(dir)
	if err != nil {
		return err
	}
	files, err := filesManifest(dir, contents)
	if err != nil {
		return err
	}
	root, err := putBlob(dir, mustMarshal(index{
		SchemaVersion: 2,
		MediaType:     indexType,
		ArtifactType:  ArtifactType,
		Manifests:     append(indexFile.Manifests, files),
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
			Annotations:  map[string]string{refNameAnnotation: contents.Config.Name + ":" + contents.Config.Version},
		}},
	})
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
	entry, err := filesEntry(bundleIndex)
	if err != nil {
		return nil, err
	}
	body, err = readBlob(dir, entry.Digest)
	if err != nil {
		return nil, err
	}
	var filesManifest manifest
	if err := json.Unmarshal(body, &filesManifest); err != nil {
		return nil, fmt.Errorf("decode the files manifest: %w", err)
	}
	config, err := readBlob(dir, filesManifest.Config.Digest)
	if err != nil {
		return nil, err
	}
	var decoded Config
	if err := json.Unmarshal(config, &decoded); err != nil {
		return nil, fmt.Errorf("decode the bundle config: %w", err)
	}
	if len(filesManifest.Layers) != 1 {
		return nil, fmt.Errorf("the bundle manifest carries %d layers, want 1: %w", len(filesManifest.Layers), ErrBundleShape)
	}
	archive, err := readBlob(dir, filesManifest.Layers[0].Digest)
	if err != nil {
		return nil, err
	}
	files, err := unpackFiles(archive)
	if err != nil {
		return nil, err
	}
	evidence, err := readEvidence(dir, root)
	if err != nil {
		return nil, err
	}
	return &Bundle{Config: decoded, Files: files, Evidence: evidence, LayoutDir: dir, Root: root.Digest}, nil
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

// mustMarshal encodes a value vessel just built, so it cannot carry an unencodable field.
func mustMarshal(value any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
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
