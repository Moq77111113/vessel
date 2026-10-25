package registry

import (
	"fmt"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

const refNameAnnotation = "org.opencontainers.image.ref.name"

func WriteLayout(dir string, ref delivery.Ref, image v1.Image) error {
	path, err := layout.FromPath(dir)
	if err != nil {
		path, err = layout.Write(dir, empty.Index)
		if err != nil {
			return fmt.Errorf("create the layout in %s: %w", dir, err)
		}
	}
	annotations := layout.WithAnnotations(map[string]string{refNameAnnotation: ref.String()})
	if err := path.AppendImage(image, annotations); err != nil {
		return fmt.Errorf("write %s into the layout: %w", ref, err)
	}
	return nil
}
