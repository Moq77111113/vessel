package link

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Moq77111113/vessel/internal/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
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
	Evidence         []string
}

// Signing returns the signer a release build uses, and fails before any image is resolved.
type Signing func(context.Context) (attest.ArtifactSigner, error)

func Build(ctx context.Context, work report.Report, stdout io.Writer, kinds []machine.Machine, job Job, signing Signing) error {
	fromBundle, err := bundleSource(job)
	if err != nil {
		return err
	}
	if !fromBundle && len(job.Evidence) > 0 {
		return ErrEvidenceNeedsBundle
	}

	var signer attest.ArtifactSigner
	if !job.InsecureUnsigned {
		if signer, err = signing(ctx); err != nil {
			return err
		}
	}

	var dir, layout string
	var cleanup func()
	if fromBundle {
		dir, layout, cleanup, err = bundleLayouts(job)
	} else {
		dir, layout, cleanup, err = linkLayouts(ctx, work, kinds, job)
	}
	if err != nil {
		return err
	}
	defer cleanup()
	if err := attachEvidence(dir, job.Evidence); err != nil {
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
	return publishSigned(ctx, stdout, temp, job.Out, layout, signer)
}

// Errors Build returns for a bundle given as its source.
var (
	ErrBundleSourceFlags   = errors.New("a bundle already carries its name, version, platform and layout, drop those flags")
	ErrSigningMismatch     = errors.New("the bundle says how it is signed, build it the same way")
	ErrEvidenceNeedsBundle = errors.New("evidence describes a bundle, build the layout first, then build from it with --evidence")
)

// bundleSource reports whether the job names a bundle, and refuses one its flags contradict.
func bundleSource(job Job) (bool, error) {
	artifact, err := bundle.Open(job.Source)
	if errors.Is(err, bundle.ErrNotABundle) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if job.Name != "" || job.Version != "" || job.Platform != "" || job.Layout != "" {
		return false, ErrBundleSourceFlags
	}
	if artifact.Config.Insecure != job.InsecureUnsigned {
		return false, fmt.Errorf("%s says insecure=%v: %w", job.Source, artifact.Config.Insecure, ErrSigningMismatch)
	}
	return true, nil
}

// bundleLayouts packs and signs the bundle the job names, as it is on disk.
func bundleLayouts(job Job) (pack, sign string, cleanup func(), err error) {
	return job.Source, job.Source, func() {}, dropLayoutSignature(job.Source)
}

// linkLayouts links the descriptor the job names into a layout, kept when the job asks for one.
func linkLayouts(ctx context.Context, work report.Report, kinds []machine.Machine, job Job) (pack, sign string, cleanup func(), err error) {
	dir, cleanup, err := layoutDir(job.Layout)
	if err != nil {
		return "", "", nil, err
	}
	if err := dropLayoutSignature(dir); err != nil {
		cleanup()
		return "", "", nil, err
	}
	if err := Link(ctx, work, report.New(io.Discard), kinds, job, dir); err != nil {
		cleanup()
		return "", "", nil, err
	}
	return dir, job.Layout, cleanup, nil
}

// attachEvidence reads each file the job names and attaches it to the layout under its base name.
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

// layouts returns the layout to pack and the one to sign: the bundle the job names, or one linked from its descriptor.
func layouts(ctx context.Context, work report.Report, kinds []machine.Machine, job Job) (string, string, func(), error) {
	if bundle.IsBundle(job.Source) {
		if err := checkBundleSource(job); err != nil {
			return "", "", nil, err
		}
		return job.Source, job.Source, func() {}, dropLayoutSignature(job.Source)
	}
	if len(job.Evidence) > 0 {
		return "", "", nil, ErrEvidenceNeedsBundle
	}
	dir, cleanup, err := layoutDir(job.Layout)
	if err != nil {
		return "", "", nil, err
	}
	if err := dropLayoutSignature(dir); err != nil {
		cleanup()
		return "", "", nil, err
	}
	if err := Link(ctx, work, report.New(io.Discard), kinds, job, dir); err != nil {
		cleanup()
		return "", "", nil, err
	}
	return dir, job.Layout, cleanup, nil
}

// checkBundleSource refuses flags a bundle already answers, and a signing mode its config contradicts.
func checkBundleSource(job Job) error {
	if job.Name != "" || job.Version != "" || job.Layout != "" {
		return ErrBundleSourceFlags
	}
	artifact, err := bundle.Open(job.Source)
	if err != nil {
		return err
	}
	if artifact.Config.Insecure != job.InsecureUnsigned {
		return fmt.Errorf("%s says insecure=%v: %w", job.Source, artifact.Config.Insecure, ErrSigningMismatch)
	}
	return nil
}

// signLayout signs the index.json of the layout the operator keeps, and leaves the signature beside it.
func signLayout(ctx context.Context, layout string, signer attest.ArtifactSigner) error {
	if layout == "" {
		return nil
	}
	_, err := attest.SignFile(ctx, filepath.Join(layout, "index.json"), signer)
	return err
}

// dropLayoutSignature removes a signature an earlier build left, since this build changes what it covers.
func dropLayoutSignature(layout string) error {
	err := os.Remove(filepath.Join(layout, "index.json"+attest.SigstoreSuffix))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("remove the old layout signature: %w", err)
}

// Sigstore signs through Fulcio with the CI's OIDC identity, and fails fast without one.
func Sigstore(ctx context.Context) (attest.ArtifactSigner, error) {
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

// publishSigned signs the executable, then the kept layout, and only then moves the executable into place.
func publishSigned(ctx context.Context, stdout io.Writer, temp, out, layout string, signer attest.ArtifactSigner) error {
	sidecar, err := attest.SignFile(ctx, temp, signer)
	if err != nil {
		return err
	}
	if err := signLayout(ctx, layout, signer); err != nil {
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
	if layout != "" {
		fmt.Fprintf(stdout, "%s, verify the layout against it\n", filepath.Join(layout, "index.json"+attest.SigstoreSuffix))
	}
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
