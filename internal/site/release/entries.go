package release

import (
	"slices"

	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

// Entries names every file this install puts down, with the digest of what it wrote.
func Entries(files []delivery.File) []Entry {
	entries := make([]Entry, 0, len(files))
	for _, file := range files {
		entries = append(entries, Entry{
			Path:   file.Path,
			Digest: disk.DigestOf(file.Data),
		})
	}
	return entries
}

// Union names every file the machine holds for this delivery and every file the new version carries.
func Union(previous, next []Entry) []Entry {
	entries := slices.Clone(next)
	for _, entry := range previous {
		if !slices.ContainsFunc(next, func(candidate Entry) bool { return candidate.Path == entry.Path }) {
			entries = append(entries, entry)
		}
	}
	return entries
}
