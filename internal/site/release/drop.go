package release

import (
	"slices"

	"github.com/Moq77111113/vessel/internal/site/disk"
)

func Drops(tree *disk.Tree, previous, next []Entry) (remove, keep []string, err error) {
	for _, entry := range previous {
		if Holds(next, entry.Path) {
			continue
		}
		change, err := tree.Differs(entry.Path, entry.Digest)
		if err != nil {
			return nil, nil, err
		}
		if change {
			keep = append(keep, entry.Path)
			continue
		}
		remove = append(remove, entry.Path)
	}
	slices.Sort(remove)
	slices.Sort(keep)
	return remove, keep, nil
}
