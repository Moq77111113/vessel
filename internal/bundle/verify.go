package bundle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Moq77111113/vessel/internal/attest"
)

// publicKey is stamped in at build time and is the only key a bundle is trusted against.
//
//	go build -ldflags "-X github.com/Moq77111113/vessel/internal/bundle.publicKey=$(cat vessel.pub)"
var publicKey string

var ErrNoPublicKey = errors.New("this build carries no public key, it cannot verify a signed bundle")

var ErrBundleHasNoName = errors.New("this bundle carries no name, build it again with this vessel")

// OpenVerified opens a bundle an operator handed over, checking its signature first.
func OpenVerified(dir string) (*Bundle, error) {
	if err := verifySignature(dir); err != nil {
		return nil, err
	}
	return OpenNamed(dir)
}

// OpenNamed opens a bundle and refuses one carrying no name.
func OpenNamed(dir string) (*Bundle, error) {
	artifact, err := Open(dir)
	if err != nil {
		return nil, err
	}
	if artifact.Config.Name == "" {
		return nil, fmt.Errorf("%s: %w", dir, ErrBundleHasNoName)
	}
	return artifact, nil
}

// verifySignature checks the detached signature beside a bundle against this build's key.
func verifySignature(dir string) error {
	root, err := RootBytes(dir)
	if err != nil {
		return err
	}
	signature, err := os.ReadFile(filepath.Join(dir, SignatureName))
	if err != nil {
		if publicKey == "" {
			return nil
		}
		return fmt.Errorf("read the bundle signature: %w", err)
	}
	if publicKey == "" {
		return ErrNoPublicKey
	}
	verifier, err := attest.NewVerifier(publicKey)
	if err != nil {
		return err
	}
	return verifier.Verify(root, signature)
}
