package link

import (
	"fmt"
	"os"
	"path/filepath"

	"aead.dev/minisign"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
)

func signBundle(dir string, key *minisign.PrivateKey) error {
	if key == nil {
		return nil
	}
	root, err := bundle.RootBytes(dir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, bundle.SignatureName)
	if err := os.WriteFile(path, attest.Sign(*key, root), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// signFile writes a detached minisign signature beside path, the way the minisign tool does.
func signFile(path string, key *minisign.PrivateKey) error {
	if key == nil {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	signature := path + ".minisig"
	if err := os.WriteFile(signature, attest.Sign(*key, body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", signature, err)
	}
	return nil
}

// resolveKey opens and parses a minisign private key, or answers no key when path is empty.
func resolveKey(path string) (*minisign.PrivateKey, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the private key: %w", err)
	}
	key, err := attest.ReadKey(data, os.Getenv("VESSEL_KEY_PASSWORD"))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &key, nil
}
