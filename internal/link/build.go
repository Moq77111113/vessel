package link

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

type Job struct {
	Source           string
	Out              string
	Layout           string
	Platform         string
	Name             string
	Version          string
	InsecureUnsigned bool
}

func Build(ctx context.Context, work report.Report, stdout io.Writer, kinds []machine.Machine, job Job) error {
	var signer attest.ArtifactSigner
	if !job.InsecureUnsigned {
		var err error
		if signer, err = preflightSigner(ctx); err != nil {
			return err
		}
	}

	dir, cleanup, err := layoutDir(job.Layout)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := Link(ctx, work, report.New(io.Discard), kinds, job.Source, dir, job.Platform, job.Name, job.Version); err != nil {
		return err
	}

	stub, err := openSelf()
	if err != nil {
		return err
	}
	defer stub.Close()

	temp, err := tempNear(job.Out)
	if err != nil {
		return err
	}
	defer os.Remove(temp)
	defer os.Remove(temp + attest.SigstoreSuffix)

	if err := installer.Pack(stub, dir, temp, work); err != nil {
		return err
	}
	if job.InsecureUnsigned {
		return publishInsecure(stdout, temp, job.Out)
	}
	return publishSigned(ctx, stdout, temp, job.Out, signer)
}

// preflightSigner fails fast on a missing CI identity, before any image is resolved.
func preflightSigner(ctx context.Context) (attest.ArtifactSigner, error) {
	tokens := attest.NewOIDCTokenSource(os.Getenv, nil)
	if _, err := tokens.Token(ctx); err != nil {
		return nil, fmt.Errorf("sign vessel releases by default: %w", err)
	}
	return attest.NewSigstoreSigner(tokens, attest.ProductionSigstoreServiceConfig), nil
}

func layoutDir(layout string) (string, func(), error) {
	if layout != "" {
		return layout, func() {}, nil
	}
	temp, err := os.MkdirTemp("", "vessel-bundle-*")
	if err != nil {
		return "", nil, fmt.Errorf("build a bundle in a temporary directory: %w", err)
	}
	return temp, func() { os.RemoveAll(temp) }, nil
}

func openSelf() (*os.File, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find this binary: %w", err)
	}
	stub, err := os.Open(self)
	if err != nil {
		return nil, fmt.Errorf("read this binary: %w", err)
	}
	return stub, nil
}

// tempNear reserves a unique name next to out so the final rename stays on one filesystem.
func tempNear(out string) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(out), ".vessel-build-*")
	if err != nil {
		return "", fmt.Errorf("create a temporary file next to %s: %w", out, err)
	}
	name := file.Name()
	file.Close()
	os.Remove(name)
	return name, nil
}

// publishInsecure moves the unsigned artifact into place; no sidecar is ever written.
func publishInsecure(stdout io.Writer, temp, out string) error {
	if err := os.Rename(temp, out); err != nil {
		return fmt.Errorf("publish %s: %w", out, err)
	}
	if err := announceFinished(stdout, out); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "INSECURE: %s is unsigned, development use only\n", out)
	return nil
}

// publishSigned signs the packed bytes, then moves the installer and its sidecar into place.
func publishSigned(ctx context.Context, stdout io.Writer, temp, out string, signer attest.ArtifactSigner) error {
	sidecar, err := attest.SignFile(ctx, temp, signer)
	if err != nil {
		return err
	}
	if err := os.Rename(temp, out); err != nil {
		return fmt.Errorf("publish %s: %w", out, err)
	}
	if err := os.Rename(sidecar, out+attest.SigstoreSuffix); err != nil {
		return fmt.Errorf("publish %s: %w", out+attest.SigstoreSuffix, err)
	}
	if err := announceFinished(stdout, out); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s, ship it alongside\n", out+attest.SigstoreSuffix)
	return nil
}

func announceFinished(stdout io.Writer, out string) error {
	info, err := os.Stat(out)
	if err != nil {
		return fmt.Errorf("read %s: %w", out, err)
	}
	report.New(stdout).Line("Finished", fmt.Sprintf("%s, %d MB, run it on the target machine", out, info.Size()/(1<<20)))
	return nil
}
