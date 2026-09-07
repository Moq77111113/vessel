package descriptor

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// ErrNoDescriptor says no reader recognized the source directory.
var ErrNoDescriptor = errors.New("no descriptor found")

// Reader is the port every descriptor format implements.
type Reader interface {
	Name() string
	Detect(fs.FS) bool
	Read(fs.FS) (Manifest, error)
	// Start returns the commands that bring these files up, in order.
	Start(files []File) []string
	// Requires names what the machine must already hold for these files to start.
	Requires(files []File) []string
	// Owns reports whether a path under the target root belongs to this reader's units.
	Owns(path string) bool
}

// ByName returns the reader that produced a bundle, so load can ask it how to start.
func ByName(readers []Reader, name string) (Reader, error) {
	for _, candidate := range readers {
		if candidate.Name() == name {
			return candidate, nil
		}
	}
	return nil, fmt.Errorf("%w: this build cannot read %q", ErrNoDescriptor, name)
}

// Pick returns the first reader that recognizes the directory, trying them in order.
func Pick(readers []Reader, dir fs.FS) (Reader, error) {
	for _, candidate := range readers {
		if candidate.Detect(dir) {
			return candidate, nil
		}
	}
	names := make([]string, len(readers))
	for i, candidate := range readers {
		names[i] = candidate.Name()
	}
	return nil, fmt.Errorf("%w, formats tried: %s", ErrNoDescriptor, strings.Join(names, ", "))
}
