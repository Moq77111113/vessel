package values

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetRefusesAnEmptyValue(t *testing.T) {
	if _, err := ParseSet([]string{"PUBLIC_HOST="}); !errors.Is(err, ErrSetIsEmpty) {
		t.Errorf("got %v, want ErrSetIsEmpty", err)
	}
}

func TestReadSetFileTakesTheValueWithoutItsNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db-password")
	if err := os.WriteFile(path, []byte("hunter2\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	values, err := ReadSetFile([]string{"DB_PASSWORD=" + path})
	if err != nil {
		t.Fatalf("ReadSetFile: %v", err)
	}
	if values["DB_PASSWORD"] != "hunter2" {
		t.Errorf("got %q, want %q", values["DB_PASSWORD"], "hunter2")
	}
}

func TestReadSetFileRefusesAFileOtherUsersCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db-password")
	if err := os.WriteFile(path, []byte("hunter2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadSetFile([]string{"DB_PASSWORD=" + path}); !errors.Is(err, ErrSecretFileOpen) {
		t.Fatalf("got %v, want ErrSecretFileOpen", err)
	}
}
