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

	"github.com/Moq77111113/vessel/pkg/delivery"
)

var (
	ErrPlatform  = errors.New("unreadable platform")
	ErrRefSyntax = errors.New("unreadable image reference")
)

type Client struct{}

func New() *Client { return &Client{} }

func (c *Client) Resolve(ctx context.Context, ref delivery.Ref, platform string) (string, error) {
	options, err := remoteOptions(ctx, platform)
	if err != nil {
		return "", err
	}
	target, err := name.ParseReference(ref.String())
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", ref, ErrRefSyntax, err)
	}
	manifest, err := remote.Get(target, options...)
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

func (c *Client) Image(ctx context.Context, ref delivery.Ref) (v1.Image, error) {
	target, err := name.ParseReference(ref.String())
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", ref, ErrRefSyntax, err)
	}
	options, err := remoteOptions(ctx, "")
	if err != nil {
		return nil, err
	}
	image, err := remote.Image(target, options...)
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
	spec, err := v1.ParsePlatform(platform)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", platform, ErrPlatform)
	}
	return append(options, remote.WithPlatform(*spec)), nil
}
