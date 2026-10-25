package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Moq77111113/vessel/internal/build/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
)

type layouts struct {
	pack    string
	sign    string
	cleanup func()
}

func bundleSource(req Request) (bool, error) {
	artifact, err := bundle.Open(req.Source)
	if errors.Is(err, bundle.ErrNotABundle) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if req.Name != "" || req.Version != "" || req.Platform != "" || req.Layout != "" {
		return false, ErrBundleSourceFlags
	}
	if artifact.Config.Insecure != req.InsecureUnsigned {
		return false, fmt.Errorf("%s says insecure=%v: %w", req.Source, artifact.Config.Insecure, ErrSigningMismatch)
	}
	return true, nil
}

func bundleLayouts(req Request) (layouts, error) {
	return layouts{pack: req.Source, sign: req.Source, cleanup: func() {}}, dropLayoutSignature(req.Source)
}

func (b Build) linkLayouts(ctx context.Context, req Request) (layouts, error) {
	place, err := scratch(req.Layout)
	if err != nil {
		return layouts{}, err
	}
	if err := dropLayoutSignature(place.pack); err != nil {
		place.cleanup()
		return layouts{}, err
	}
	_, err = Link{Drivers: b.Drivers}.Run(ctx, LinkRequest{
		Source:           req.Source,
		Name:             req.Name,
		Version:          req.Version,
		Platform:         req.Platform,
		InsecureUnsigned: req.InsecureUnsigned,
		Dir:              place.pack,
		Work:             req.Work,
	})
	if err != nil {
		place.cleanup()
		return layouts{}, err
	}
	return place, nil
}

func scratch(layout string) (layouts, error) {
	if layout != "" {
		return layouts{pack: layout, sign: layout, cleanup: func() {}}, nil
	}
	temp, err := os.MkdirTemp("", "vessel-bundle-*")
	if err != nil {
		return layouts{}, fmt.Errorf("build a bundle in a temporary directory: %w", err)
	}
	return layouts{pack: temp, cleanup: func() { os.RemoveAll(temp) }}, nil
}

func attachEvidence(dir string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	evidence := make([]bundle.Evidence, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read the evidence %s: %w", path, err)
		}
		evidence = append(evidence, bundle.Evidence{Name: filepath.Base(path), Data: data})
	}
	return bundle.Attach(dir, evidence)
}

func dropLayoutSignature(layout string) error {
	err := os.Remove(filepath.Join(layout, "index.json"+attest.SignatureSuffix))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("remove the old layout signature: %w", err)
}
