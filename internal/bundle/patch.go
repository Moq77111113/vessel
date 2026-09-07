package bundle

import (
	"errors"
	"fmt"
	"sort"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// ErrNoDigest says a relocation was left without the digest it needs.
var ErrNoDigest = errors.New("no digest")

// Patch rewrites every relocation into its digest form and returns the resulting files.
func Patch(manifest descriptor.Manifest, digests map[string]string) ([]descriptor.File, error) {
	files := make([]descriptor.File, len(manifest.Files))
	for i, file := range manifest.Files {
		files[i] = descriptor.File{Path: file.Path, Data: append([]byte(nil), file.Data...)}
	}

	relocs := append([]descriptor.Relocation(nil), manifest.Relocs...)
	sort.Slice(relocs, func(a, b int) bool {
		if relocs[a].File != relocs[b].File {
			return relocs[a].File > relocs[b].File
		}
		return relocs[a].Offset > relocs[b].Offset
	})

	for _, reloc := range relocs {
		digest, ok := digests[reloc.Ref.String()]
		if !ok {
			return nil, fmt.Errorf("%w for %s", ErrNoDigest, reloc.Ref)
		}
		data := files[reloc.File].Data
		replacement := []byte(reloc.Ref.WithDigest(digest).String())
		patch := make([]byte, 0, len(data)-reloc.Length+len(replacement))
		patch = append(patch, data[:reloc.Offset]...)
		patch = append(patch, replacement...)
		patch = append(patch, data[reloc.Offset+reloc.Length:]...)
		files[reloc.File].Data = patch
	}
	return files, nil
}
