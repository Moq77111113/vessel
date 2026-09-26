package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type blob struct {
	MediaType   string            `json:"mediaType"`
	Digest      string            `json:"digest"`
	Size        int64             `json:"size"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	ArtifactType  string `json:"artifactType,omitempty"`
	Config        blob   `json:"config"`
	Layers        []blob `json:"layers"`
	Subject       blob   `json:"subject,omitzero"`
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

// filesManifest writes the config and the descriptor files, and returns the manifest over them.
func filesManifest(dir string, contents Contents) (entry, error) {
	archive, err := packFiles(contents.Files)
	if err != nil {
		return entry{}, err
	}
	layer, err := putBlob(dir, archive)
	if err != nil {
		return entry{}, err
	}
	config, err := putBlob(dir, mustMarshal(contents.Config))
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

// bundleEntry finds the bundle root by reading each candidate index, not by trusting the
// entry that points at it: a registry round trip drops artifactType from the entry while
// the blob it names keeps it.
func bundleEntry(dir string) (entry, error) {
	var indexFile index
	if err := readJSON(filepath.Join(dir, indexName), &indexFile); err != nil {
		return entry{}, fmt.Errorf("%s %w", dir, ErrNotABundle)
	}
	var corruption error
	for _, candidate := range indexFile.Manifests {
		if candidate.MediaType != indexType {
			continue
		}
		body, err := readBlob(dir, candidate.Digest)
		if errors.Is(err, ErrDigestMismatch) {
			corruption = err
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
	if corruption != nil {
		return entry{}, corruption
	}
	return entry{}, fmt.Errorf("%s %w", dir, ErrNotABundle)
}

func readIndex(dir string) (index, error) {
	var indexFile index
	if err := readJSON(filepath.Join(dir, indexName), &indexFile); err != nil {
		return index{}, err
	}
	return indexFile, nil
}

func writeIndex(dir string, indexFile index) error {
	path := filepath.Join(dir, indexName)
	if err := os.WriteFile(path, mustMarshal(indexFile), 0o644); err != nil {
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
	return entry{}, fmt.Errorf("the bundle index holds no %s manifest: %w", FilesType, ErrBundleShape)
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
