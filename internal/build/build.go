package build

import (
	"context"
	"errors"
	"io"

	"github.com/Moq77111113/vessel/internal/build/attest"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/driver"
)

var (
	ErrBundleSourceFlags   = errors.New("a bundle already carries its name, version, platform and layout, drop those flags")
	ErrSigningMismatch     = errors.New("the bundle says how it is signed, build it the same way")
	ErrEvidenceNeedsBundle = errors.New("evidence describes a bundle, build the layout first, then build from it with --evidence")
)

type Build struct {
	Drivers []driver.Driver
}

type Request struct {
	Source           string
	Out              string
	Layout           string
	Platform         string
	Name             string
	Version          string
	InsecureUnsigned bool
	Evidence         []string
	Key              string
	Work             report.Report
	Stdout           io.Writer
}

type Result struct {
	Executable string
	Signature  string
}

func (b Build) Run(ctx context.Context, req Request) (Result, error) {
	fromBundle, err := bundleSource(req)
	if err != nil {
		return Result{}, err
	}
	if !fromBundle && len(req.Evidence) > 0 {
		return Result{}, ErrEvidenceNeedsBundle
	}
	key, err := signingKey(req)
	if err != nil {
		return Result{}, err
	}
	var place layouts
	if fromBundle {
		place, err = bundleLayouts(req)
	} else {
		place, err = b.linkLayouts(ctx, req)
	}
	if err != nil {
		return Result{}, err
	}
	defer place.cleanup()
	if err := attachEvidence(place.pack, req.Evidence); err != nil {
		return Result{}, err
	}
	binary, err := req.executable(place.pack)
	if err != nil {
		return Result{}, err
	}
	defer binary.discard()
	if req.InsecureUnsigned {
		return binary.publishInsecure(req.Stdout)
	}
	return binary.publishSigned(req.Stdout, key, place.sign)
}

func signingKey(req Request) (*attest.Key, error) {
	if req.InsecureUnsigned {
		return nil, nil
	}
	return attest.ReadKey(req.Key)
}
