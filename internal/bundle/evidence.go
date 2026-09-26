package bundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
)

const (
	// EvidenceType marks the manifest carrying the SBOMs and reports a CI attached to a bundle.
	EvidenceType = "application/vnd.vessel.evidence.v1"
	evidenceFile = "application/vnd.vessel.evidence.file.v1"
	// OCI 1.1 empty descriptor: an artifact manifest with no config points at the two bytes "{}".
	emptyType = "application/vnd.oci.empty.v1+json"
	titleKey  = "org.opencontainers.image.title"
)

// Evidence is one file a CI made from a bundle: an SBOM, a scan report.
type Evidence struct {
	Name string
	Data []byte
}

// Attach adds evidence to the bundle at dir, bound to its root; the same file attached again is kept once.
func Attach(dir string, evidence []Evidence) error {
	root, err := bundleEntry(dir)
	if err != nil {
		return err
	}
	present, err := readEvidence(dir, root)
	if err != nil {
		return err
	}
	fresh, err := freshEvidence(present, evidence)
	if err != nil || len(fresh) == 0 {
		return err
	}
	return attach(dir, root, fresh)
}

// freshEvidence keeps the files the bundle does not carry yet, refusing a name taken by other content.
func freshEvidence(present, evidence []Evidence) ([]Evidence, error) {
	carried := make(map[string][]byte, len(present))
	for _, file := range present {
		carried[file.Name] = file.Data
	}
	incoming := make(map[string]bool, len(evidence))
	var fresh []Evidence
	for _, file := range evidence {
		if err := checkEvidenceName(file.Name); err != nil {
			return nil, err
		}
		if incoming[file.Name] {
			return nil, fmt.Errorf("%s: %w", file.Name, ErrEvidenceSameName)
		}
		incoming[file.Name] = true
		data, ok := carried[file.Name]
		if ok && bytes.Equal(data, file.Data) {
			continue
		}
		if ok {
			return nil, fmt.Errorf("%s: %w", file.Name, ErrEvidenceTwice)
		}
		fresh = append(fresh, file)
	}
	return fresh, nil
}

// attach writes the evidence manifest bound to root and lists it in index.json beside the root.
func attach(dir string, root entry, evidence []Evidence) error {
	config, err := putBlob(dir, []byte("{}"))
	if err != nil {
		return err
	}
	layers := make([]blob, 0, len(evidence))
	for _, file := range evidence {
		layer, err := putBlob(dir, file.Data)
		if err != nil {
			return err
		}
		layers = append(layers, blob{MediaType: evidenceFile, Digest: layer.Digest, Size: layer.Size,
			Annotations: map[string]string{titleKey: file.Name}})
	}
	body, err := putBlob(dir, mustMarshal(manifest{
		SchemaVersion: 2,
		MediaType:     manifestType,
		ArtifactType:  EvidenceType,
		Config:        blob{MediaType: emptyType, Digest: config.Digest, Size: config.Size},
		Layers:        layers,
		Subject:       blob{MediaType: indexType, Digest: root.Digest, Size: root.Size},
	}))
	if err != nil {
		return err
	}
	indexFile, err := readIndex(dir)
	if err != nil {
		return err
	}
	indexFile.Manifests = append(indexFile.Manifests, entry{
		MediaType: manifestType, ArtifactType: EvidenceType, Digest: body.Digest, Size: body.Size,
	})
	return writeIndex(dir, indexFile)
}

// readEvidence returns every file the evidence manifests bound to root carry, refusing a name that is a path.
func readEvidence(dir string, root entry) ([]Evidence, error) {
	indexFile, err := readIndex(dir)
	if err != nil {
		return nil, err
	}
	var evidence []Evidence
	names := map[string]bool{}
	for _, candidate := range indexFile.Manifests {
		if candidate.MediaType != manifestType {
			continue
		}
		body, err := readBlob(dir, candidate.Digest)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var decoded manifest
		if err := json.Unmarshal(body, &decoded); err != nil {
			return nil, fmt.Errorf("decode %s: %w", candidate.Digest, err)
		}
		if decoded.ArtifactType != EvidenceType || decoded.Subject.Digest != root.Digest {
			continue
		}
		for _, layer := range decoded.Layers {
			name := layer.Annotations[titleKey]
			if err := checkEvidenceName(name); err != nil {
				return nil, err
			}
			if names[name] {
				return nil, fmt.Errorf("%s: %w", name, ErrEvidenceTwice)
			}
			names[name] = true
			data, err := readBlob(dir, layer.Digest)
			if err != nil {
				return nil, err
			}
			evidence = append(evidence, Evidence{Name: name, Data: data})
		}
	}
	return evidence, nil
}

// checkEvidenceName refuses a name that is empty, a dot entry, or anything but one path segment.
func checkEvidenceName(name string) error {
	if name == "" || name == "." || name == ".." || path.Base(name) != name {
		return fmt.Errorf("%q: %w", name, ErrEvidenceName)
	}
	return nil
}
