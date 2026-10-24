// Package attesttest writes the signing key every test that signs starts from.
package attesttest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

// Key writes a fresh PKCS#8 PEM ECDSA P-256 private key and returns its path.
func Key(t *testing.T) string {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate a key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatalf("encode the key: %v", err)
	}
	path := filepath.Join(t.TempDir(), "vessel.key")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write the key: %v", err)
	}
	return path
}

// Verifies reports whether path+".sig" is the signature of the file at path under the key at key.
func Verifies(t *testing.T, key, path string) bool {
	t.Helper()
	body, err := os.ReadFile(key)
	if err != nil {
		t.Fatalf("read the key: %v", err)
	}
	block, _ := pem.Decode(body)
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("decode the key: %v", err)
	}
	encoded, err := os.ReadFile(path + ".sig")
	if err != nil {
		t.Fatalf("read the signature: %v", err)
	}
	signature, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		t.Fatalf("decode the signature: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	digest := sha256.Sum256(data)
	return ecdsa.VerifyASN1(&parsed.(*ecdsa.PrivateKey).PublicKey, digest[:], signature)
}
