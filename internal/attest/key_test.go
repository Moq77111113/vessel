package attest

import (
	"errors"
	"sync"
	"testing"

	"aead.dev/minisign"
)

// lockedKey is built once: minisign encrypts with scrypt, which costs seconds per call.
var lockedKey = sync.OnceValues(func() ([]byte, error) {
	_, private, err := minisign.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	return minisign.EncryptKey(lockedPassword, private)
})

const lockedPassword = "hunter2"

func lockedKeyFile(t *testing.T) []byte {
	t.Helper()
	text, err := lockedKey()
	if err != nil {
		t.Fatalf("EncryptKey: %v", err)
	}
	return text
}

func openKeyFile(t *testing.T) []byte {
	t.Helper()
	_, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	text, err := private.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	return text
}

func TestReadKeyOpensAKeyWithoutAPassword(t *testing.T) {
	if _, err := ReadKey(openKeyFile(t), ""); err != nil {
		t.Fatalf("ReadKey: %v", err)
	}
}

func TestReadKeyOpensAKeyWithItsPassword(t *testing.T) {
	if _, err := ReadKey(lockedKeyFile(t), lockedPassword); err != nil {
		t.Fatalf("ReadKey: %v", err)
	}
}

func TestReadKeyRefusesTheWrongPassword(t *testing.T) {
	if _, err := ReadKey(lockedKeyFile(t), "wrong"); !errors.Is(err, ErrBadKey) {
		t.Errorf("got %v, want ErrBadKey", err)
	}
}

func TestReadKeySaysHowToSupplyAMissingPassword(t *testing.T) {
	_, err := ReadKey(lockedKeyFile(t), "")
	if !errors.Is(err, ErrKeyPassword) {
		t.Fatalf("got %v, want ErrKeyPassword", err)
	}
	if got := err.Error(); got == "" {
		t.Error("the error says nothing")
	}
}

func TestReadKeyRefusesSomethingThatIsNotAKey(t *testing.T) {
	if _, err := ReadKey([]byte("hello"), ""); !errors.Is(err, ErrBadKey) {
		t.Errorf("got %v, want ErrBadKey", err)
	}
}

func TestAKeyReadWithItsPasswordStillSigns(t *testing.T) {
	key, err := ReadKey(lockedKeyFile(t), lockedPassword)
	if err != nil {
		t.Fatalf("ReadKey: %v", err)
	}
	root := []byte(`{"name":"acme"}`)
	verifier, err := NewVerifier(key.Public().(minisign.PublicKey).String())
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	if err := verifier.Verify(root, Sign(key, root)); err != nil {
		t.Errorf("Verify: %v", err)
	}
}
