package link

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

// Job is one build run: the source it reads and everything it writes.
type Job struct {
	Source   string
	Out      string
	Layout   string
	Platform string
	Name     string
	Version  string
	Key      string
}

// Build resolves the descriptor into a bundle, then folds that bundle into one executable.
func Build(ctx context.Context, work report.Report, stdout io.Writer, kinds []machine.Machine, job Job) error {
	out := job.Out
	key, err := resolveKey(job.Key)
	if err != nil {
		return err
	}
	dir := job.Layout
	if dir == "" {
		temp, err := os.MkdirTemp("", "vessel-bundle-*")
		if err != nil {
			return fmt.Errorf("build a bundle in a temporary directory: %w", err)
		}
		defer os.RemoveAll(temp)
		dir = temp
	}

	if err := Link(ctx, work, report.New(io.Discard), kinds, job.Source, dir, job.Platform, job.Name, job.Version, key); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find this binary: %w", err)
	}
	stub, err := os.Open(self)
	if err != nil {
		return fmt.Errorf("read this binary: %w", err)
	}
	defer stub.Close()

	if err := installer.Pack(stub, dir, out, work); err != nil {
		return err
	}
	if err := signFile(out, key); err != nil {
		return err
	}
	info, err := os.Stat(out)
	if err != nil {
		return fmt.Errorf("read %s: %w", out, err)
	}
	report.New(stdout).Line("Finished", fmt.Sprintf("%s, %d MB, run it on the target machine", out, info.Size()/(1<<20)))
	if key != nil {
		fmt.Fprintf(stdout, "%s.minisig, ship it alongside\n", out)
		return nil
	}
	fmt.Fprintf(stdout, "unsigned: the operator has no way to tell this file is yours.\n"+
		"Sign it with --key, or print `sha256sum %s` on the install sheet.\n", filepath.Base(out))
	return nil
}
