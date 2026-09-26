package attest

import (
	"context"
	"fmt"
	"os"

	"github.com/Moq77111113/vessel/internal/atomicfile"
)

// SigstoreSuffix names the sidecar file holding a release's Sigstore bundle.
const SigstoreSuffix = ".sigstore"

// ArtifactSigner turns artifact bytes into a Sigstore bundle.
type ArtifactSigner interface {
	Sign(context.Context, []byte) ([]byte, error)
}

// SignFile signs the artifact at path and atomically writes the bundle to path+SigstoreSuffix.
func SignFile(ctx context.Context, path string, signer ArtifactSigner) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read installer: %w", err)
	}

	bundle, err := signer.Sign(ctx, body)
	if err != nil {
		return "", fmt.Errorf("sign installer: %w", err)
	}

	sidecar := path + SigstoreSuffix
	if err := atomicfile.Write(sidecar, bundle, 0o644); err != nil {
		return "", fmt.Errorf("write signature: %w", err)
	}
	return sidecar, nil
}
