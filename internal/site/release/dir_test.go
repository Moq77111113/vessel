package release

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

func TestDirJoinsTheNameUnderVarLibVessel(t *testing.T) {
	root := t.TempDir()
	got, err := Dir(root, "acme")
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	want := filepath.Join(root, "var/lib/vessel", "acme")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDirRefusesANameThatLeavesTheTargetRoot(t *testing.T) {
	_, err := Dir(t.TempDir(), "../../../etc/cron.daily")
	if !errors.Is(err, delivery.ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}
