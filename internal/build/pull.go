package build

import (
	"context"
	"sort"

	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/Moq77111113/vessel/internal/build/registry"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

type puller interface {
	Resolve(ctx context.Context, ref delivery.Ref, platform string) (string, error)
	Image(ctx context.Context, ref delivery.Ref) (v1.Image, error)
}

type pull struct {
	client   puller
	work     report.Report
	platform string
	layout   string
}

func (p pull) resolve(ctx context.Context, relocs []delivery.Relocation) (map[string]string, error) {
	digests := make(map[string]string, len(relocs))
	for _, reloc := range relocs {
		tag := reloc.Ref.String()
		if _, seen := digests[tag]; seen {
			continue
		}
		p.work.Line("Resolving", tag)
		digest, err := p.client.Resolve(ctx, reloc.Ref, p.platform)
		if err != nil {
			return nil, err
		}
		ref := reloc.Ref.WithDigest(digest)
		p.work.Line("Pulling", ref.String())
		image, err := p.client.Image(ctx, ref)
		if err != nil {
			return nil, err
		}
		if err := registry.WriteLayout(p.layout, ref, image); err != nil {
			return nil, err
		}
		digests[tag] = digest
	}
	return digests, nil
}

func images(digests map[string]string) []bundle.Image {
	list := make([]bundle.Image, 0, len(digests))
	for ref, digest := range digests {
		list = append(list, bundle.Image{Ref: ref, Digest: digest})
	}
	sort.Slice(list, func(a, b int) bool { return list[a].Ref < list[b].Ref })
	return list
}
