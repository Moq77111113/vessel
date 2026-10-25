package release

import (
	"slices"

	"github.com/Moq77111113/vessel/internal/site/disk"
)

type Drop struct {
	Remove []string
	Keep   []string
}

func Drops(tree *disk.Tree, previous, next []Entry) (Drop, error) {
	var drop Drop
	for _, entry := range previous {
		if Holds(next, entry.Path) {
			continue
		}
		change, err := tree.Differs(entry.Path, entry.Digest)
		if err != nil {
			return Drop{}, err
		}
		if change {
			drop.Keep = append(drop.Keep, entry.Path)
			continue
		}
		drop.Remove = append(drop.Remove, entry.Path)
	}
	slices.Sort(drop.Remove)
	slices.Sort(drop.Keep)
	return drop, nil
}
