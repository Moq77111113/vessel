package descriptor

import (
	"errors"
	"fmt"
	"strings"
)

// Errors ParseRef returns when a reference cannot be read.
var (
	ErrEmptyRef   = errors.New("empty image reference")
	ErrNoRegistry = errors.New("image reference carries no registry")
)

// Ref is an image reference, split so a digest can take the place of the tag.
type Ref struct {
	Registry   string
	Repository string
	Tag        string
	Digest     string
}

// ParseRef reads a registry/repository:tag reference, or one already pinned to a digest.
func ParseRef(s string) (Ref, error) {
	if s == "" {
		return Ref{}, ErrEmptyRef
	}
	registry, rest, ok := strings.Cut(s, "/")
	if !ok {
		return Ref{}, fmt.Errorf("%q: %w", s, ErrNoRegistry)
	}
	if repository, digest, ok := strings.Cut(rest, "@"); ok {
		return Ref{Registry: registry, Repository: repository, Digest: digest}, nil
	}
	repository, tag, ok := strings.Cut(rest, ":")
	if !ok {
		return Ref{Registry: registry, Repository: rest, Tag: "latest"}, nil
	}
	return Ref{Registry: registry, Repository: repository, Tag: tag}, nil
}

// WithDigest returns the same reference pinned to a digest.
func (r Ref) WithDigest(digest string) Ref {
	r.Digest = digest
	r.Tag = ""
	return r
}

func (r Ref) String() string {
	if r.Digest != "" {
		return fmt.Sprintf("%s/%s@%s", r.Registry, r.Repository, r.Digest)
	}
	return fmt.Sprintf("%s/%s:%s", r.Registry, r.Repository, r.Tag)
}
