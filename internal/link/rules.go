package link

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
)

// ErrTargetCollides says a delivery file claims a path a unit already occupies.
var ErrTargetCollides = errors.New("a file target collides with a unit")

// ErrTargetDuplicate says two delivery files claim the same path.
var ErrTargetDuplicate = errors.New("a file target is declared twice")

// ErrSecretNoUnitReads says a delivery declares a secret no unit ever reads. The podman secret
// vessel creates carries the variable name, and Secret= is the only way a container sees it.
var ErrSecretNoUnitReads = errors.New("no unit reads this secret with a Secret= key")

// ErrTargetInUnitDirectory says a delivery file lands where the reader writes its units. A file
// carried there is never read as a unit, so its image is never pinned to a digest.
var ErrTargetInUnitDirectory = errors.New("a file target lands in the unit directory")

// checkNoCollision refuses a plain file whose path a unit already occupies, or that another
// plain file already claims.
func checkNoCollision(units, files []descriptor.File) error {
	unitTargets := make(map[string]bool, len(units))
	for _, unit := range units {
		unitTargets[unit.Path] = true
	}
	targets := make(map[string]bool, len(files))
	for _, file := range files {
		if unitTargets[file.Path] {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetCollides)
		}
		if targets[file.Path] {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetDuplicate)
		}
		targets[file.Path] = true
	}
	return nil
}

// checkOutsideTheUnitDirectories refuses a plain file that lands where the reader writes its
// units: link never reads it as a unit, so nothing pins the image it may name.
func checkOutsideTheUnitDirectories(kind machine.Descriptor, files []descriptor.File) error {
	for _, file := range files {
		if kind.Owns(file.Path) {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetInUnitDirectory)
		}
	}
	return nil
}

// checkEverySecretIsRead refuses a secret variable no unit reads: vessel creates the podman secret
// under the variable name, and a unit reaches it with a Secret= key naming that same name.
func checkEverySecretIsRead(required []string, variables []descriptor.Variable) error {
	for _, name := range descriptor.SecretNames(variables) {
		if !slices.Contains(required, name) {
			return fmt.Errorf("%s: %w", name, ErrSecretNoUnitReads)
		}
	}
	return nil
}
