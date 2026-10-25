package delivery

import (
	"errors"
	"fmt"
)

// ErrTargetCollides says a delivery file claims a path a unit already occupies.
var ErrTargetCollides = errors.New("a file target collides with a unit")

// ErrTargetDuplicate says two delivery files claim the same path.
var ErrTargetDuplicate = errors.New("a file target is declared twice")

// CheckCollisions refuses a plain file whose path a unit already occupies, or that another
// plain file already claims.
func CheckCollisions(units, files []File) error {
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
