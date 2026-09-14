package delivery

import (
	"path/filepath"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// deliveriesPath is where a machine keeps what it holds for each delivery, under the target root.
const deliveriesPath = "var/lib/vessel"

// deliveriesDir is where a machine keeps every delivery it holds, under the target root.
func deliveriesDir(root string) string {
	return filepath.Join(root, deliveriesPath)
}

// dirFor is the directory a machine keeps one delivery under, or refuses its name.
func dirFor(root, name string) (string, error) {
	if err := descriptor.CheckName(name); err != nil {
		return "", err
	}
	return filepath.Join(deliveriesDir(root), name), nil
}
