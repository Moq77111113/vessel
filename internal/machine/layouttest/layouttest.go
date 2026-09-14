// Package layouttest writes the OCI layout every test that reads one starts from.
package layouttest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Moq77111113/vessel/internal/machine"
)

// TwoImages writes an OCI layout holding two images that share one layer, and returns its directory.
func TwoImages(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	blobs := filepath.Join(dir, "blobs", "sha256")
	if err := os.MkdirAll(blobs, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	put := func(body []byte) string {
		sum := sha256.Sum256(body)
		digest := hex.EncodeToString(sum[:])
		if err := os.WriteFile(filepath.Join(blobs, digest), body, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		return "sha256:" + digest
	}
	layer := put([]byte("a layer both images carry"))

	var manifests []map[string]any
	for _, name := range []string{"registry.example.com/acme/web:1.0", "registry.example.com/library/postgres:17.2"} {
		config := put([]byte(`{"architecture":"amd64","os":"linux","name":"` + name + `"}`))
		body, err := json.Marshal(map[string]any{
			"schemaVersion": 2,
			"mediaType":     "application/vnd.oci.image.manifest.v1+json",
			"config":        map[string]any{"mediaType": "application/vnd.oci.image.config.v1+json", "digest": config, "size": 1},
			"layers":        []map[string]any{{"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip", "digest": layer, "size": 1}},
		})
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		digest := put(body)
		manifests = append(manifests, map[string]any{
			"mediaType":   "application/vnd.oci.image.manifest.v1+json",
			"digest":      digest,
			"size":        len(body),
			"annotations": map[string]string{machine.RefNameAnnotation: name},
		})
	}
	index, err := json.Marshal(map[string]any{"schemaVersion": 2, "manifests": manifests})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	write(t, filepath.Join(dir, "index.json"), index)
	write(t, filepath.Join(dir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`))
	return dir
}

// Open returns the layout TwoImages writes, already open.
func Open(t *testing.T) *machine.Layout {
	t.Helper()
	layout, err := machine.OpenLayout(TwoImages(t))
	if err != nil {
		t.Fatalf("OpenLayout: %v", err)
	}
	return layout
}

func write(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}
