package delivery

import (
	"slices"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/record"
)

// entriesOf names every file this install puts down, with the digest of what it wrote.
func entriesOf(files []descriptor.File) []record.Entry {
	entries := make([]record.Entry, 0, len(files))
	for _, file := range files {
		entries = append(entries, record.Entry{
			Path:   file.Path,
			Digest: machine.DigestOf(file.Data),
		})
	}
	return entries
}

// digestsOf names the digest an install resolved each image to.
func digestsOf(images []bundle.Image) []string {
	digests := make([]string, 0, len(images))
	for _, image := range images {
		digests = append(digests, image.Digest)
	}
	return digests
}

// namesOf names the secrets this install creates, in a stable order.
func namesOf(secrets map[string]string) []string {
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// unionNames merges two name lists, sorted and without duplicates.
func unionNames(previous, next []string) []string {
	names := make(map[string]bool, len(previous)+len(next))
	for _, name := range previous {
		names[name] = true
	}
	for _, name := range next {
		names[name] = true
	}
	all := make([]string, 0, len(names))
	for name := range names {
		all = append(all, name)
	}
	slices.Sort(all)
	return all
}

// union names every file the machine holds for this delivery and every file the new version carries.
func union(previous, next []record.Entry) []record.Entry {
	entries := slices.Clone(next)
	for _, entry := range previous {
		if !slices.ContainsFunc(next, func(candidate record.Entry) bool { return candidate.Path == entry.Path }) {
			entries = append(entries, entry)
		}
	}
	return entries
}
