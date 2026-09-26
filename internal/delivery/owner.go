package delivery

import (
	"errors"
	"fmt"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/record"
)

// Errors an install returns for a file it does not own, before it changes anything.
var (
	ErrFileExists = errors.New("is already on this machine and no delivery put it there, move it away or drop it from the descriptor")
	ErrFileTaken  = errors.New("belongs to another delivery")
)

// claim refuses a file this install would write that another delivery owns, or that no delivery put there.
func (i Install) claim(current record.Record, files []descriptor.File) error {
	others, err := owners(i.Root, i.Artifact.Config.Name)
	if err != nil {
		return err
	}
	tree := machine.NewTree(i.Root)
	for _, file := range files {
		if names(current.Files, file.Path) {
			continue
		}
		if delivery, ok := others[file.Path]; ok {
			return fmt.Errorf("/%s %w, %s", file.Path, ErrFileTaken, delivery)
		}
		holds, err := tree.Holds(file.Path)
		if err != nil {
			return err
		}
		if holds {
			return fmt.Errorf("/%s %w", file.Path, ErrFileExists)
		}
	}
	return nil
}

// owners maps every file another delivery's record names to that delivery.
func owners(root, name string) (map[string]string, error) {
	deliveries, err := record.Deliveries(deliveriesDir(root))
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, other := range deliveries {
		if other == name {
			continue
		}
		dir, err := dirFor(root, other)
		if err != nil {
			return nil, err
		}
		entry, _, err := record.NewRecords(dir).Read()
		if err != nil {
			return nil, err
		}
		for _, file := range entry.Files {
			paths[file.Path] = other
		}
	}
	return paths, nil
}

// names reports whether entries name path.
func names(entries []record.Entry, path string) bool {
	for _, entry := range entries {
		if entry.Path == path {
			return true
		}
	}
	return false
}
