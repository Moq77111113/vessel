package delivery

import (
	"errors"
	"fmt"
)

// ErrTargetCollides says a delivery file claims a path a file the driver reads already occupies.
var ErrTargetCollides = errors.New("a file target collides with a file the driver reads")

// ErrTargetDuplicate says two delivery files claim the same path.
var ErrTargetDuplicate = errors.New("a file target is declared twice")

// CheckCollisions refuses a plain file whose path a file the driver read occupies, or another plain file claims.
func CheckCollisions(read, files []File) error {
	readTargets := make(map[string]bool, len(read))
	for _, file := range read {
		readTargets[file.Path] = true
	}
	targets := make(map[string]bool, len(files))
	for _, file := range files {
		if readTargets[file.Path] {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetCollides)
		}
		if targets[file.Path] {
			return fmt.Errorf("%s: %w", file.Path, ErrTargetDuplicate)
		}
		targets[file.Path] = true
	}
	return nil
}
