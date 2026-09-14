package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/quadlet"
)

func TestAMachineThatIsNotReadyExitsWithThree(t *testing.T) {
	err := fmt.Errorf("podman is too old: %w", quadlet.ErrNotReady)
	if got := Code(err); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestAFailureThatTouchedNothingExitsWithFour(t *testing.T) {
	if got := Code(errors.New("a delivery this build refuses")); got != 4 {
		t.Errorf("got %d, want 4", got)
	}
}

func TestAFailureAfterTheRecordIsOpenExitsWithFive(t *testing.T) {
	err := fmt.Errorf("%w: no file could be written", delivery.ErrPartlyInstalled)
	if got := Code(err); got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}
