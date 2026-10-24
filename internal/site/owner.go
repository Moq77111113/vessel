package site

import (
	"errors"
	"fmt"

	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

var (
	ErrFileExists = errors.New("is already on this machine and no delivery put it there, move it away or drop it from the descriptor")
	ErrFileTaken  = errors.New("belongs to another delivery")
)

func (i Install) claim(name string, current release.Record, files []delivery.File) error {
	others, err := release.Owners(i.Root, name)
	if err != nil {
		return err
	}
	tree := disk.NewTree(i.Root)
	for _, file := range files {
		if release.Holds(current.Files, file.Path) {
			continue
		}
		if owner, ok := others[file.Path]; ok {
			return fmt.Errorf("/%s %w, %s", file.Path, ErrFileTaken, owner)
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
