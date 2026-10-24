package build

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Moq77111113/vessel/internal/atomicfile"
	"github.com/Moq77111113/vessel/internal/build/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
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

func Build(ctx context.Context, work report.Report, stdout io.Writer, kinds []driver.Driver, job Job) error {
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

type executable struct {
	temp   string
	out    string
	digest []byte
}

func pack(stub io.Reader, dir, out string, work report.Report) (executable, error) {
	temp, err := os.CreateTemp(filepath.Dir(out), ".vessel-build-*")
	if err != nil {
		return executable{}, fmt.Errorf("create a temporary file next to %s: %w", out, err)
	}
	hash := sha256.New()
	err = Pack(stub, dir, io.MultiWriter(temp, hash), work)
	if err == nil {
		err = temp.Chmod(0o755)
	}
	if err := errors.Join(err, temp.Close()); err != nil {
		os.Remove(temp.Name())
		return executable{}, err
	}
	return executable{temp: temp.Name(), out: out, digest: hash.Sum(nil)}, nil
}

var (
	ErrBundleSourceFlags   = errors.New("a bundle already carries its name, version, platform and layout, drop those flags")
	ErrSigningMismatch     = errors.New("the bundle says how it is signed, build it the same way")
	ErrEvidenceNeedsBundle = errors.New("evidence describes a bundle, build the layout first, then build from it with --evidence")
)

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

func bundleLayouts(job Job) (pack, sign string, cleanup func(), err error) {
	return job.Source, job.Source, func() {}, dropLayoutSignature(job.Source)
}

func linkLayouts(ctx context.Context, work report.Report, kinds []driver.Driver, job Job) (pack, sign string, cleanup func(), err error) {
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

func (e executable) publishInsecure(stdout io.Writer) error {
	if err := e.move(stdout); err != nil {
		return err
	}
	fmt.Fprintln(stdout, bundle.InsecureWarning)
	return nil
}

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

var ErrSecretNotRead = errors.New("no service reads this secret")

var ErrTargetInDriverDirectory = errors.New("a file target lands in a directory the driver fills")

func checkOutsideTheDriverDirectories(dirs []string, files []delivery.File) error {
	for _, file := range files {
		for _, dir := range dirs {
			if strings.HasPrefix(path.Clean(file.Path), dir+"/") {
				return fmt.Errorf("%s: %w", file.Path, ErrTargetInDriverDirectory)
			}
		}
	}
	return nil
}

func checkEverySecretIsRead(read []string, variables []delivery.Variable) error {
	for _, name := range delivery.SecretNames(variables) {
		if !slices.Contains(read, name) {
			return fmt.Errorf("%s: %w", name, ErrSecretNotRead)
		}
	}
	return nil
}
