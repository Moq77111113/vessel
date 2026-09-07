package target

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValuesReadsBackWhatItWrote(t *testing.T) {
	values := NewValues(t.TempDir(), "acme")
	if err := values.Write(map[string]string{"PUBLIC_HOST": "dmas.acme.local"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := values.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %v, want PUBLIC_HOST=dmas.acme.local", got)
	}
}

func TestValuesReadsNothingOnAMachineWithNoStore(t *testing.T) {
	got, err := NewValues(t.TempDir(), "acme").Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

func TestValuesWritesAFileNoOtherUserCanRead(t *testing.T) {
	root := t.TempDir()
	if err := NewValues(root, "acme").Write(map[string]string{"A": "x"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "var/lib/vessel/acme/values"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("got mode %o, want 600", info.Mode().Perm())
	}
}

func TestValuesKeepsAValueHoldingAnEqualsSign(t *testing.T) {
	values := NewValues(t.TempDir(), "acme")
	if err := values.Write(map[string]string{"A": "x=y=z"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := values.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got["A"] != "x=y=z" {
		t.Errorf("got %q, want %q", got["A"], "x=y=z")
	}
}

func TestValuesRefusesAValueHoldingANewline(t *testing.T) {
	values := NewValues(t.TempDir(), "acme")
	err := values.Write(map[string]string{"A": "one\ntwo"})
	if !errors.Is(err, ErrValueHasANewline) {
		t.Fatalf("got %v, want ErrValueHasANewline", err)
	}
}

func TestValuesRefusalLeavesNoFileBehind(t *testing.T) {
	root := t.TempDir()
	if err := NewValues(root, "acme").Write(map[string]string{"A": "one\ntwo"}); !errors.Is(err, ErrValueHasANewline) {
		t.Fatalf("got %v, want ErrValueHasANewline", err)
	}
	if _, err := os.Stat(filepath.Join(root, "var/lib/vessel/acme/values")); !os.IsNotExist(err) {
		t.Fatalf("got %v, want the store file to not exist", err)
	}
}

func TestValuesRefusalDoesNotLoseAnEarlierSuccessfulWrite(t *testing.T) {
	values := NewValues(t.TempDir(), "acme")
	if err := values.Write(map[string]string{"PUBLIC_HOST": "dmas.acme.local"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := values.Write(map[string]string{"A": "one\ntwo"}); !errors.Is(err, ErrValueHasANewline) {
		t.Fatalf("got %v, want ErrValueHasANewline", err)
	}
	got, err := values.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %v, want the earlier write's PUBLIC_HOST=dmas.acme.local", got)
	}
}
