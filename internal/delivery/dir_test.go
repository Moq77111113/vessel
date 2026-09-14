package delivery

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

func TestDirForJoinsTheNameUnderVarLibVessel(t *testing.T) {
	root := t.TempDir()
	got, err := dirFor(root, "acme")
	if err != nil {
		t.Fatalf("dirFor: %v", err)
	}
	want := filepath.Join(root, "var/lib/vessel", "acme")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDirForRefusesANameThatLeavesTheTargetRoot(t *testing.T) {
	_, err := dirFor(t.TempDir(), "../../../etc/cron.daily")
	if !errors.Is(err, descriptor.ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}
