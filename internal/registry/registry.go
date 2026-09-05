// Package registry resolves image references and pulls them into an OCI layout.
package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// refNameAnnotation is the key podman reads to name an image it loads.
const refNameAnnotation = "org.opencontainers.image.ref.name"

// Errors a client returns before it ever reaches a registry.
var (
	ErrPlatform  = errors.New("unreadable platform")
	ErrRefSyntax = errors.New("unreadable image reference")
)

// Client reads registries over the network.
type Client struct{}

// New returns a client that reads public and authenticated registries.
func New() *Client { return &Client{} }

// Resolve returns the digest a reference points at right now.
func (c *Client) Resolve(ctx context.Context, ref descriptor.Ref, platform string) (string, error) {
	options, err := remoteOptions(ctx, platform)
	if err != nil {
		return "", err
	}
	parsed, err := name.ParseReference(ref.String())
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", ref, ErrRefSyntax, err)
	}
	image, err := remote.Get(parsed, options...)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	return image.Digest.String(), nil
}

// Fetch pulls a pinned reference into the OCI layout at dir, under its whole name.
func (c *Client) Fetch(ctx context.Context, ref descriptor.Ref, dir string) error {
	parsed, err := name.ParseReference(ref.String())
	if err != nil {
		return fmt.Errorf("%s: %w: %s", ref, ErrRefSyntax, err)
	}
	image, err := remote.Image(parsed, remote.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	path, err := layout.FromPath(dir)
	if err != nil {
		path, err = layout.Write(dir, emptyIndex())
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

func remoteOptions(ctx context.Context, platform string) ([]remote.Option, error) {
	options := []remote.Option{remote.WithContext(ctx)}
	if platform == "" {
		return options, nil
	}
	if fields := strings.Split(platform, "/"); len(fields) < 2 || fields[0] == "" || fields[1] == "" {
		return nil, fmt.Errorf("%q: %w, want os/arch", platform, ErrPlatform)
	}
	parsed, err := v1.ParsePlatform(platform)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", platform, ErrPlatform)
	}
	return append(options, remote.WithPlatform(*parsed)), nil
}
