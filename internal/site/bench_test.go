package site

import (
	"context"
	"io"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/pkg/driver"
)

type bench struct {
	t     *testing.T
	root  string
	kinds []driver.Driver
}

func at(t *testing.T, root string, kinds []driver.Driver) bench {
	if kinds == nil {
		kinds = []driver.Driver{quadlet.New(root, (&podmanStub{}).run)}
	}
	return bench{t: t, root: root, kinds: kinds}
}

func (b bench) job(dir string, set map[string]string) testJob {
	b.t.Helper()
	artifact, err := bundle.OpenNamed(dir)
	if err != nil {
		b.t.Fatalf("open the bundle: %v", err)
	}
	install := Install{Root: b.root, Drivers: b.kinds, Shell: disk.NewShell(noCapture)}
	return testJob{Install: install, Artifact: artifact, Set: set}
}

func (b bench) install(dir string) error {
	b.t.Helper()
	return b.job(dir, nil).Run(context.Background(), io.Discard, report.New(io.Discard))
}

func (b bench) upgrade(dir string) error {
	b.t.Helper()
	return b.job(dir, nil).Upgrade(context.Background(), io.Discard, report.New(io.Discard))
}

func (b bench) resume(dir string) error {
	b.t.Helper()
	return b.job(dir, nil).Resume(context.Background(), io.Discard, report.New(io.Discard))
}

func (b bench) status(out io.Writer, name string) error {
	_, err := Status{Root: b.root, Drivers: b.kinds}.Run(context.Background(), StatusRequest{Name: name, Out: out})
	return err
}

func (b bench) uninstall(out io.Writer, name string) error {
	_, err := Uninstall{Root: b.root, Drivers: b.kinds}.Run(context.Background(), UninstallRequest{Name: name, Out: out})
	return err
}

type testJob struct {
	Install
	Artifact *bundle.Bundle
	Set      map[string]string
	SetFile  map[string]string
}

func (j testJob) Preview() testJob {
	j.Install = j.Install.Preview()
	return j
}

func (j testJob) request(out io.Writer, work report.Report) InstallRequest {
	return InstallRequest{Artifact: j.Artifact, Set: j.Set, SetFile: j.SetFile, Out: out, Work: work}
}

func (j testJob) Run(ctx context.Context, out io.Writer, work report.Report) error {
	_, err := j.Install.Run(ctx, j.request(out, work))
	return err
}

func (j testJob) Upgrade(ctx context.Context, out io.Writer, work report.Report) error {
	_, err := Upgrade(j.Install).Run(ctx, j.request(out, work))
	return err
}

func (j testJob) Resume(ctx context.Context, out io.Writer, work report.Report) error {
	_, err := Resume(j.Install).Run(ctx, j.request(out, work))
	return err
}

func (j testJob) SkipAction(ctx context.Context, out io.Writer, work report.Report) error {
	_, err := SkipAction(j.Install).Run(ctx, j.request(out, work))
	return err
}
