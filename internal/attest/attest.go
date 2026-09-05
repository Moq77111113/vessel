// Package attest signs the root of a bundle and verifies it offline.
package attest

import (
	"errors"
	"fmt"

	"aead.dev/minisign"
)

// Errors a verifier returns when trust does not hold.
var (
	ErrBadKey       = errors.New("unreadable public key")
	ErrBadSignature = errors.New("signature does not cover this bundle")
)

// Sign returns a detached minisign signature over the bundle root.
func Sign(key minisign.PrivateKey, root []byte) []byte {
	return minisign.Sign(key, root)
}

// Verifier checks a bundle signature against one public key.
type Verifier struct {
	key minisign.PublicKey
}

// NewVerifier reads a minisign public key in its text form.
func NewVerifier(publicKey string) (*Verifier, error) {
	var key minisign.PublicKey
	if err := key.UnmarshalText([]byte(publicKey)); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrBadKey, err)
	}
	return &Verifier{key: key}, nil
}

// Verify reports whether the signature covers exactly these root bytes.
func (v *Verifier) Verify(root, signature []byte) error {
	if !minisign.Verify(v.key, root, signature) {
		return ErrBadSignature
	}
	return nil
}
