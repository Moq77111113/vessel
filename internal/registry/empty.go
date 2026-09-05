package registry

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
)

// empty is the starting index a fresh OCI layout carries.
func emptyIndex() v1.ImageIndex { return empty.Index }
