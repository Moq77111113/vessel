package machine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Moq77111113/vessel/internal/delivery"
)

func TestSiteReadsBackWhatItWrote(t *testing.T) {
	values := siteFor(t, t.TempDir(), "acme")
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
	got, err := siteFor(t, t.TempDir(), "acme").Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

func TestSitePathNamesWhereTheValuesLive(t *testing.T) {
	root := t.TempDir()
	got := siteFor(t, root, "acme").Path()
	want := filepath.Join(root, "var/lib/vessel/acme/values")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSiteWritesAFileNoOtherUserCanRead(t *testing.T) {
	root := t.TempDir()
	if err := siteFor(t, root, "acme").Write(map[string]string{"A": "x"}); err != nil {
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

func TestSiteKeepsAValueHoldingAnEqualsSign(t *testing.T) {
	values := siteFor(t, t.TempDir(), "acme")
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
	values := siteFor(t, t.TempDir(), "acme")
	err := values.Write(map[string]string{"A": "one\ntwo"})
	if !errors.Is(err, ErrValueHasANewline) {
		t.Fatalf("got %v, want ErrValueHasANewline", err)
	}
}

func TestSiteRefusalLeavesNoFileBehind(t *testing.T) {
	root := t.TempDir()
	if err := siteFor(t, root, "acme").Write(map[string]string{"A": "one\ntwo"}); !errors.Is(err, ErrValueHasANewline) {
		t.Fatalf("got %v, want ErrValueHasANewline", err)
	}
	if _, err := os.Stat(filepath.Join(root, "var/lib/vessel/acme/values")); !os.IsNotExist(err) {
		t.Fatalf("got %v, want the store file to not exist", err)
	}
}

func TestSiteRefusalDoesNotLoseAnEarlierSuccessfulWrite(t *testing.T) {
	values := siteFor(t, t.TempDir(), "acme")
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
	root := t.TempDir()
	if err := siteFor(t, root, "acme").Write(map[string]string{"A": "x"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "var/lib/vessel/acme"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if got, want := len(entries), 1; got != want {
		t.Errorf("entries: got %d, want %d", got, want)
	}
}

func TestNewSiteRefusesANameThatLeavesTheTargetRoot(t *testing.T) {
	_, err := NewSite(t.TempDir(), "../../../etc/cron.daily")
	if !errors.Is(err, delivery.ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func siteFor(t *testing.T, root, name string) *Site {
	t.Helper()
	site, err := NewSite(root, name)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}
	return site
}
