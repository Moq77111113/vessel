package site

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSiteReadsBackWhatItWrote(t *testing.T) {
	values := siteFor(t, t.TempDir())
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

func TestSiteReadsNothingOnAMachineWithNoStore(t *testing.T) {
	got, err := siteFor(t, t.TempDir()).Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

func TestSitePathNamesWhereTheValuesLive(t *testing.T) {
	dir := t.TempDir()
	got := siteFor(t, dir).Path()
	want := filepath.Join(dir, "values")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSiteWritesAFileNoOtherUserCanRead(t *testing.T) {
	dir := t.TempDir()
	if err := siteFor(t, dir).Write(map[string]string{"A": "x"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "values"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("got mode %o, want 600", info.Mode().Perm())
	}
}

func TestSiteKeepsAValueHoldingAnEqualsSign(t *testing.T) {
	values := siteFor(t, t.TempDir())
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

func TestSiteRefusesAValueHoldingANewline(t *testing.T) {
	values := siteFor(t, t.TempDir())
	err := values.Write(map[string]string{"A": "one\ntwo"})
	if !errors.Is(err, ErrValueHasANewline) {
		t.Fatalf("got %v, want ErrValueHasANewline", err)
	}
}

func TestSiteRefusalLeavesNoFileBehind(t *testing.T) {
	dir := t.TempDir()
	if err := siteFor(t, dir).Write(map[string]string{"A": "one\ntwo"}); !errors.Is(err, ErrValueHasANewline) {
		t.Fatalf("got %v, want ErrValueHasANewline", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "values")); !os.IsNotExist(err) {
		t.Fatalf("got %v, want the store file to not exist", err)
	}
}

func TestSiteRefusalDoesNotLoseAnEarlierSuccessfulWrite(t *testing.T) {
	values := siteFor(t, t.TempDir())
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

func TestSiteWriteLeavesNoTemporaryFileBehind(t *testing.T) {
	dir := t.TempDir()
	if err := siteFor(t, dir).Write(map[string]string{"A": "x"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if got, want := len(entries), 1; got != want {
		t.Errorf("entries: got %d, want %d", got, want)
	}
}

func siteFor(t *testing.T, dir string) *Store {
	t.Helper()
	return NewStore(dir)
}
