package build

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

// ErrSecretNoUnitReads says a delivery declares a secret no unit ever reads. The podman secret
// vessel creates carries the variable name, and Secret= is the only way a container sees it.
var ErrSecretNoUnitReads = errors.New("no unit reads this secret with a Secret= key")

// ErrTargetInUnitDirectory says a delivery file lands where the reader writes its units. A file
// carried there is never read as a unit, so its image is never pinned to a digest.
var ErrTargetInUnitDirectory = errors.New("a file target lands in the unit directory")

// checkOutsideTheUnitDirectories refuses a plain file that lands where the reader writes its
// units: link never reads it as a unit, so nothing pins the image it may name.
func checkOutsideTheUnitDirectories(kind driver.Descriptor, files []delivery.File) error {
	for _, file := range files {
		if kind.Owns(file.Path) {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetInUnitDirectory)
		}
	}
	return nil
}

// checkEverySecretIsRead refuses a secret variable no unit reads: vessel creates the podman secret
// under the variable name, and a unit reaches it with a Secret= key naming that same name.
func checkEverySecretIsRead(required []string, variables []delivery.Variable) error {
	for _, name := range delivery.SecretNames(variables) {
		if !slices.Contains(required, name) {
			return fmt.Errorf("%s: %w", name, ErrSecretNoUnitReads)
		}
	}
	return nil
}
