package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/v1/layout"
)

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
	err = writeIndex(dir, index{
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
	if err != nil {
		return err
	}
	return prune(dir)
}

func prune(dir string) error {
	oci := layout.Path(dir)
	unused, err := oci.GarbageCollect()
	if err != nil {
		return fmt.Errorf("find the blobs no index reaches: %w", err)
	}
	for _, hash := range unused {
		if err := oci.RemoveBlob(hash); err != nil {
			return fmt.Errorf("remove %s: %w", hash, err)
		}
	}
	return nil
}

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
