package build

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Moq77111113/vessel/internal/atomicfile"
	"github.com/Moq77111113/vessel/internal/build/attest"
	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
)

type executable struct {
	temp   string
	out    string
	digest []byte
}

func (r Request) executable(dir string) (executable, error) {
	stub, err := openSelf()
	if err != nil {
		return executable{}, err
	}
	defer stub.Close()
	temp, err := os.CreateTemp(filepath.Dir(r.Out), ".vessel-build-*")
	if err != nil {
		return executable{}, fmt.Errorf("create a temporary file next to %s: %w", r.Out, err)
	}
	hash := sha256.New()
	err = packer{work: r.Work}.pack(stub, dir, io.MultiWriter(temp, hash))
	if err == nil {
		err = temp.Chmod(0o755)
	}
	if err := errors.Join(err, temp.Close()); err != nil {
		os.Remove(temp.Name())
		return executable{}, err
	}
	return executable{temp: temp.Name(), out: r.Out, digest: hash.Sum(nil)}, nil
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

func (e executable) discard() { os.Remove(e.temp) }

func (e executable) publishInsecure(stdout io.Writer) (Result, error) {
	if err := e.move(stdout); err != nil {
		return Result{}, err
	}
	fmt.Fprintln(stdout, bundle.InsecureWarning)
	return Result{Executable: e.out}, nil
}

func (e executable) publishSigned(stdout io.Writer, key *attest.Key, layout string) (Result, error) {
	signature, err := key.Sign(e.digest)
	if err != nil {
		return Result{}, err
	}
	if layout != "" {
		if err := key.SignFile(filepath.Join(layout, "index.json")); err != nil {
			return Result{}, err
		}
	}
	if err := atomicfile.Write(e.out+attest.SignatureSuffix, signature, 0o644); err != nil {
		return Result{}, err
	}
	if err := e.move(stdout); err != nil {
		return Result{}, err
	}
	fmt.Fprintf(stdout, "%s, ship it alongside\n", e.out+attest.SignatureSuffix)
	if layout != "" {
		fmt.Fprintf(stdout, "%s, verify the layout against it\n", filepath.Join(layout, "index.json"+attest.SignatureSuffix))
	}
	return Result{Executable: e.out, Signature: e.out + attest.SignatureSuffix}, nil
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
