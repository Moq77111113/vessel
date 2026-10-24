package site

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

func (i Install) fileChanges(files []delivery.File, current []release.Entry, unknown map[string]string) ([]change, error) {
	changes := make([]change, 0, len(files))
	for _, file := range files {
		changes = append(changes, fileChange(filepath.Join(i.Root, file.Path), file, unknown))
	}
	drop, err := release.Drops(disk.NewTree(i.Root), current, release.Entries(files))
	if err != nil {
		return nil, err
	}
	remove, keep := drop.Remove, drop.Keep
	for _, path := range remove {
		changes = append(changes, change{verb: "Remove", line: path})
	}
	for _, path := range keep {
		changes = append(changes, change{verb: "Keep", line: path + ", edited on this machine"})
	}
	return changes, nil
}

func fileChange(path string, file delivery.File, unknown map[string]string) change {
	var names []string
	for mark, name := range unknown {
		if bytes.Contains(file.Data, []byte(mark)) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	_, err := os.Stat(path)
	absent := os.IsNotExist(err)
	switch {
	case len(names) > 0 && absent:
		return change{verb: "Add", line: file.Path + ", content depends on " + strings.Join(names, ", ")}
	case len(names) > 0:
		return change{verb: "Depends", line: file.Path + " on " + strings.Join(names, ", ")}
	case absent:
		return change{verb: "Add", line: file.Path}
	case disk.Same(path, disk.Digest(file.Data)):
		return change{verb: "Keep", line: file.Path}
	}
	return change{verb: "Replace", line: file.Path}
}
