package build

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/Moq77111113/vessel/pkg/driver"
)

var ErrNoDriver = errors.New("no descriptor found")

func pick(drivers []driver.Driver, dir fs.FS) (driver.Driver, error) {
	for _, candidate := range drivers {
		if candidate.Detect(dir) {
			return candidate, nil
		}
	}
	names := make([]string, len(drivers))
	for i, candidate := range drivers {
		names[i] = candidate.Name()
	}
	return nil, fmt.Errorf("%w, kinds tried: %s", ErrNoDriver, strings.Join(names, ", "))
}
