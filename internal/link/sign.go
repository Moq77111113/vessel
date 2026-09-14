package link

import (
	"fmt"
	"os"
	"path/filepath"

	"aead.dev/minisign"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
)

func signBundle(dir, keyPath string) error {
	if keyPath == "" {
		return nil
	}
	key, err := readPrivateKey(keyPath)
	if err != nil {
		return err
	}
	root, err := bundle.RootBytes(dir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, bundle.SignatureName)
	if err := os.WriteFile(path, attest.Sign(key, root), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// signFile writes a detached minisign signature beside path, the way the minisign tool does.
func signFile(path, keyPath string) error {
	if keyPath == "" {
		return nil
	}
	key, err := readPrivateKey(keyPath)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	signature := path + ".minisig"
	if err := os.WriteFile(signature, attest.Sign(key, body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", signature, err)
	}
	return nil
}

// readPrivateKey opens a minisign private key, with VESSEL_KEY_PASSWORD when it has one.
func readPrivateKey(path string) (minisign.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return minisign.PrivateKey{}, fmt.Errorf("read the private key: %w", err)
	}
	key, err := attest.ReadKey(data, os.Getenv("VESSEL_KEY_PASSWORD"))
	if err != nil {
		return minisign.PrivateKey{}, fmt.Errorf("%s: %w", path, err)
	}
	return key, nil
}
