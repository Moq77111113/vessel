package attest_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/attest/attesttest"
)

func TestSignFileWritesASignatureTheKeyVerifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "myapp")
	if err := os.WriteFile(path, []byte("an executable"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	key := attesttest.Key(t)
	signer, err := attest.ReadKey(key)
	if err != nil {
		t.Fatalf("ReadKey: %v", err)
	}
	if err := signer.SignFile(path); err != nil {
		t.Fatalf("SignFile: %v", err)
	}
	if !attesttest.Verifies(t, key, path) {
		t.Error("the signature does not verify")
	}
}

func TestOpensslVerifiesTheSignature(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skipf("no openssl on this machine: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "myapp")
	if err := os.WriteFile(path, []byte("an executable"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	key := attesttest.Key(t)
	signer, err := attest.ReadKey(key)
	if err != nil {
		t.Fatalf("ReadKey: %v", err)
	}
	if err := signer.SignFile(path); err != nil {
		t.Fatalf("SignFile: %v", err)
	}
	public := filepath.Join(dir, "vessel.pub")
	der := filepath.Join(dir, "myapp.der")
	for _, args := range [][]string{
		{"pkey", "-in", key, "-pubout", "-out", public},
		{"base64", "-d", "-A", "-in", path + attest.SignatureSuffix, "-out", der},
		{"dgst", "-sha256", "-verify", public, "-signature", der, path},
	} {
		if output, err := exec.Command(openssl, args...).CombinedOutput(); err != nil {
			t.Fatalf("openssl %v: %v: %s", args, err, output)
		}
	}
}

func TestReadKeyRefusesAMissingPath(t *testing.T) {
	if _, err := attest.ReadKey(""); !errors.Is(err, attest.ErrNoKey) {
		t.Fatalf("got %v, want ErrNoKey", err)
	}
}

func TestReadKeyRefusesAFileThatIsNotAKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vessel.key")
	if err := os.WriteFile(path, []byte("not a key"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := attest.ReadKey(path); !errors.Is(err, attest.ErrKeyType) {
		t.Fatalf("got %v, want ErrKeyType", err)
	}
}
