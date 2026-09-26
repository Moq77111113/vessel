package link

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Moq77111113/vessel/internal/atomicfile"
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
	Key              string
}

func Build(ctx context.Context, work report.Report, stdout io.Writer, kinds []machine.Machine, job Job) error {
	fromBundle, err := bundleSource(job)
	if err != nil {
		return err
	}
	if !fromBundle && len(job.Evidence) > 0 {
		return ErrEvidenceNeedsBundle
	}

	var key *attest.Key
	if !job.InsecureUnsigned {
		if key, err = attest.ReadKey(job.Key); err != nil {
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

	binary, err := pack(stub, dir, job.Out, work)
	if err != nil {
		return err
	}
	defer os.Remove(binary.temp)
	if job.InsecureUnsigned {
		return binary.publishInsecure(stdout)
	}
	return binary.publishSigned(stdout, key, layout)
}

// executable is a packed file waiting beside the path it is published at, with the sha256 of its bytes.
type executable struct {
	temp   string
	out    string
	digest []byte
}

// pack writes the executable next to out through one handle, hashing exactly the bytes it writes.
func pack(stub io.Reader, dir, out string, work report.Report) (executable, error) {
	temp, err := os.CreateTemp(filepath.Dir(out), ".vessel-build-*")
	if err != nil {
		return executable{}, fmt.Errorf("create a temporary file next to %s: %w", out, err)
	}
	hash := sha256.New()
	err = installer.Pack(stub, dir, io.MultiWriter(temp, hash), work)
	if err == nil {
		err = temp.Chmod(0o755)
	}
	if err := errors.Join(err, temp.Close()); err != nil {
		os.Remove(temp.Name())
		return executable{}, err
	}
	return executable{temp: temp.Name(), out: out, digest: hash.Sum(nil)}, nil
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

// dropLayoutSignature removes a signature an earlier build left, since this build changes what it covers.
func dropLayoutSignature(layout string) error {
	err := os.Remove(filepath.Join(layout, "index.json"+attest.SignatureSuffix))
	if err == nil || os.IsNotExist(err) {
		return nil
	}
	return fmt.Errorf("remove the old layout signature: %w", err)
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

// publishInsecure moves the executable into place with no signature.
func (e executable) publishInsecure(stdout io.Writer) error {
	if err := e.move(stdout); err != nil {
		return err
	}
	fmt.Fprintln(stdout, bundle.InsecureWarning)
	return nil
}

// publishSigned signs the kept layout, writes the executable's signature, and only then moves the executable into place.
func (e executable) publishSigned(stdout io.Writer, key *attest.Key, layout string) error {
	signature, err := key.Sign(e.digest)
	if err != nil {
		return err
	}
	if layout != "" {
		if err := key.SignFile(filepath.Join(layout, "index.json")); err != nil {
			return err
		}
	}
	if err := atomicfile.Write(e.out+attest.SignatureSuffix, signature, 0o644); err != nil {
		return err
	}
	if err := e.move(stdout); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s, ship it alongside\n", e.out+attest.SignatureSuffix)
	if layout != "" {
		fmt.Fprintf(stdout, "%s, verify the layout against it\n", filepath.Join(layout, "index.json"+attest.SignatureSuffix))
	}
	return nil
}

// move renames the executable to the path it is published at, and says so.
func (e executable) move(stdout io.Writer) error {
	if err := os.Rename(e.temp, e.out); err != nil {
		return fmt.Errorf("publish %s: %w", e.out, err)
	}
	info, err := os.Stat(e.out)
	if err != nil {
		return fmt.Errorf("read %s: %w", e.out, err)
	}
	report.New(stdout).Line("Finished", fmt.Sprintf("%s, %d MB, run it on the target machine", e.out, info.Size()/(1<<20)))
	return nil
}
