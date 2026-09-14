package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
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
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc/acme"), "an ordinary file where the delivery wants a directory")
	err := load(context.Background(), io.Discard, report.New(io.Discard),
		testKinds(), machine.NewShell(noCapture), blockedFileBundle(t), root,
		map[string]string{"PUBLIC_HOST": "dmas.acme.local"}, false, modeInstall)
	if err == nil {
		t.Fatal("install succeeded even though no file could be written")
	}
	if got := Code(err); got != 5 {
		t.Errorf("got %d, want 5 for a machine the install already touched", got)
	}
}
