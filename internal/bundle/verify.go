package bundle

import (
	"errors"
	"fmt"
)

var ErrBundleHasNoName = errors.New("this bundle carries no name, build it again with this vessel")

// OpenNamed opens a bundle and requires it to carry a name.
func OpenNamed(dir string) (*Bundle, error) {
	artifact, err := Open(dir)
	if err != nil {
		return nil, err
	}
	if artifact.Config.Name == "" {
		return nil, fmt.Errorf("%s: %w", dir, ErrBundleHasNoName)
	}
	return artifact, nil
}
