package site

import (
	"slices"

	"github.com/Moq77111113/vessel/internal/bundle"
)

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
