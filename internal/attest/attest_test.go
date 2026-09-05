package attest

import (
	"errors"
	"testing"

	"aead.dev/minisign"
)

func pair(t *testing.T) (minisign.PublicKey, minisign.PrivateKey) {
	t.Helper()
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return public, private
}

func TestVerifyAcceptsASignatureOverTheSameRoot(t *testing.T) {
	public, private := pair(t)
	root := []byte(`{"name":"acme"}`)
	verifier, err := NewVerifier(public.String())
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	if err := verifier.Verify(root, Sign(private, root)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyRejectsATamperedRoot(t *testing.T) {
	public, private := pair(t)
	verifier, err := NewVerifier(public.String())
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	signature := Sign(private, []byte(`{"name":"acme"}`))
	err = verifier.Verify([]byte(`{"name":"other"}`), signature)
	if !errors.Is(err, ErrBadSignature) {
		t.Errorf("got %v, want ErrBadSignature", err)
	}
}

func TestVerifyRejectsASignatureFromAnotherKey(t *testing.T) {
	public, _ := pair(t)
	_, other := pair(t)
	root := []byte(`{"name":"acme"}`)
	verifier, err := NewVerifier(public.String())
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	if err := verifier.Verify(root, Sign(other, root)); !errors.Is(err, ErrBadSignature) {
		t.Errorf("got %v, want ErrBadSignature", err)
	}
}

func TestNewVerifierRejectsAMalformedPublicKey(t *testing.T) {
	if _, err := NewVerifier("not-a-key"); !errors.Is(err, ErrBadKey) {
		t.Errorf("got %v, want ErrBadKey", err)
	}
}

func TestNewVerifierRejectsAnEmptyPublicKey(t *testing.T) {
	if _, err := NewVerifier(""); !errors.Is(err, ErrBadKey) {
		t.Errorf("got %v, want ErrBadKey", err)
	}
}
