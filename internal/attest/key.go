// Package attest signs what vessel publishes with a key the caller provides.
package attest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Moq77111113/vessel/internal/atomicfile"
)

// SignatureSuffix names the detached signature written beside what it signs.
const SignatureSuffix = ".sig"

// Errors ReadKey returns.
var (
	ErrNoKey   = errors.New("a release build signs: pass --key, or --insecure-unsigned for development")
	ErrKeyType = errors.New("is not a PKCS#8 PEM ECDSA P-256 private key")
)

// Key is the ECDSA P-256 private key a release is signed with.
type Key struct {
	private *ecdsa.PrivateKey
}

// ReadKey reads a PKCS#8 PEM ECDSA P-256 private key, the form openssl genpkey writes.
func ReadKey(path string) (*Key, error) {
	if path == "" {
		return nil, ErrNoKey
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the key: %w", err)
	}
	block, _ := pem.Decode(body)
	if block == nil {
		return nil, fmt.Errorf("%s %w", path, ErrKeyType)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s %w: %w", path, ErrKeyType, err)
	}
	private, ok := parsed.(*ecdsa.PrivateKey)
	if !ok || private.Curve != elliptic.P256() {
		return nil, fmt.Errorf("%s %w", path, ErrKeyType)
	}
	return &Key{private: private}, nil
}

// Sign returns the base64 ASN.1 signature over a sha256 digest, the form cosign and openssl read.
func (k *Key) Sign(digest []byte) ([]byte, error) {
	signature, err := ecdsa.SignASN1(rand.Reader, k.private, digest)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	return []byte(base64.StdEncoding.EncodeToString(signature)), nil
}

// SignFile writes the signature of the file at path to path+SignatureSuffix.
func (k *Key) SignFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	signature, err := k.Sign(hash.Sum(nil))
	if err != nil {
		return err
	}
	return atomicfile.Write(path+SignatureSuffix, signature, 0o644)
}
