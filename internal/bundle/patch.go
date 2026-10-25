package bundle

import (
	"errors"
	"fmt"
	"sort"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

var ErrNoDigest = errors.New("no digest")

func Patch(manifest delivery.Manifest, digests map[string]string) ([]delivery.File, error) {
	files := make([]delivery.File, len(manifest.Files))
	for i, file := range manifest.Files {
		file.Data = append([]byte(nil), file.Data...)
		files[i] = file
	}

	relocs := append([]delivery.Relocation(nil), manifest.Relocs...)
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
