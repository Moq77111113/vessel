package bundle

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

const (
	ArtifactType = "application/vnd.vessel.bundle.v1"
	FilesType    = "application/vnd.vessel.files.manifest.v1"
	configType   = "application/vnd.vessel.bundle.config.v1+json"
	filesType    = "application/vnd.vessel.files.v1.tar"
	manifestType = "application/vnd.oci.image.manifest.v1+json"
	indexType    = "application/vnd.oci.image.index.v1+json"

	refNameAnnotation = "org.opencontainers.image.ref.name"

	indexName  = "index.json"
	layoutName = "oci-layout"
	blobsDir   = "blobs/sha256"
)

var (
	ErrDigestMismatch   = errors.New("does not match the digest that pins it")
	ErrPathEscapes      = errors.New("leaves the target root")
	ErrNotABundle       = errors.New("is not a vessel bundle")
	ErrBundleShape      = errors.New("is not shaped like a vessel bundle")
	ErrEvidenceName     = errors.New("an evidence name is not a plain file name")
	ErrEvidenceTwice    = errors.New("the bundle already carries other evidence of that name")
	ErrEvidenceSameName = errors.New("two evidence files share one name")
)

const InsecureWarning = "insecure: built with --insecure-unsigned, for development use only"

type Image struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
}

type Config struct {
	Name      string              `json:"name"`
	Version   string              `json:"version"`
	Machine   string              `json:"machine"`
	Platform  string              `json:"platform"`
	Images    []Image             `json:"images"`
	Services  []delivery.Service  `json:"services,omitempty"`
	Secrets   []string            `json:"secrets,omitempty"`
	Variables []delivery.Variable `json:"variables,omitempty"`
	Actions   []string            `json:"actions,omitempty"`
	Insecure  bool                `json:"insecure,omitempty"`
}

type Contents struct {
	Config Config
	Layout string
	Files  []delivery.File
}

type Bundle struct {
	Config    Config
	Files     []delivery.File
	Evidence  []Evidence
	LayoutDir string
	Root      string
}

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

func IsBundle(dir string) bool {
	_, err := bundleEntry(dir)
	return err == nil
}

func RootBytes(dir string) ([]byte, error) {
	root, err := bundleEntry(dir)
	if err != nil {
		return nil, err
	}
	return []byte(root.Digest), nil
}
