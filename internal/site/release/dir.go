package release

import (
	"path/filepath"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

const deliveriesPath = "var/lib/vessel"

func deliveriesDir(root string) string {
	return filepath.Join(root, deliveriesPath)
}

func Dir(root, name string) (string, error) {
	if err := delivery.CheckName(name); err != nil {
		return "", err
	}
	return filepath.Join(deliveriesDir(root), name), nil
}
