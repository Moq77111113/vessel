// Package registry pulls the images a descriptor names into an OCI layout: Client reads the
// network, WriteLayout writes the disk.
package registry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// Errors a client returns before it ever reaches a registry.
var (
	ErrPlatform  = errors.New("unreadable platform")
	ErrRefSyntax = errors.New("unreadable image reference")
)

// Client reads registries over the network.
type Client struct{}

// New returns a client that reads public registries, and private ones through the credentials
// podman and docker already keep.
func New() *Client { return &Client{} }

// Resolve returns the digest of the manifest a reference points at right now: the platform's
// manifest, not the index that may list it, so the digest names the very blob Image returns.
func (c *Client) Resolve(ctx context.Context, ref descriptor.Ref, platform string) (string, error) {
	options, err := remoteOptions(ctx, platform)
	if err != nil {
		return "", err
	}
	parsed, err := name.ParseReference(ref.String())
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", ref, ErrRefSyntax, err)
	}
	manifest, err := remote.Get(parsed, options...)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	image, err := manifest.Image()
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	digest, err := image.Digest()
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", ref, err)
	}
	return digest.String(), nil
}

// Image pulls a reference already pinned to a digest.
func (c *Client) Image(ctx context.Context, ref descriptor.Ref) (v1.Image, error) {
	parsed, err := name.ParseReference(ref.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", ref, ErrRefSyntax, err)
	}
	options, err := remoteOptions(ctx, "")
	if err != nil {
		return nil, err
	}
	image, err := remote.Image(parsed, options...)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", ref, err)
	}
	return image, nil
}

func remoteOptions(ctx context.Context, platform string) ([]remote.Option, error) {
	options := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
	}
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
