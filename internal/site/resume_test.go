package site

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/driver"
)

func TestResumeFinishesAnInstallCutAtTheImages(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	if err := at(t, root, nil).resume(dir); err != nil {
		t.Fatalf("resume: %v", err)
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !entry.Done() {
		t.Error("the record is still open after a resume that succeeded")
	}
}

func TestResumeRunsNoActionTheCutInstallFinished(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "one-action"))
	shell := &countingShell{}
	first := at(t, root, []driver.Driver{brokenImages{quadlet.New(root, (&podmanStub{}).run)}}).job(dir, nil)
	first.Shell = disk.NewShell(shell.run)
	if err := first.Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	second := at(t, root, nil).job(dir, nil)
	second.Shell = disk.NewShell(shell.run)
	if err := second.Resume(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if want := []string{"mkdir -p /srv/acme"}; !slices.Equal(shell.commands, want) {
		t.Errorf("got %v, want the action run once", shell.commands)
	}
}

func TestResumeRunsAgainAnActionThatFailed(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "one-action"))
	if err := at(t, root, nil).install(dir); err == nil {
		t.Fatal("the install succeeded with an action that failed")
	}
	shell := &countingShell{}
	job := at(t, root, nil).job(dir, nil)
	job.Shell = disk.NewShell(shell.run)
	if err := job.Resume(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if want := []string{"mkdir -p /srv/acme"}; !slices.Equal(shell.commands, want) {
		t.Errorf("got %v, want the failed action run again", shell.commands)
	}
}

func TestResumeRefusesAMachineWhoseLastInstallFinished(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := at(t, root, nil).install(dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := at(t, root, nil).resume(dir); !errors.Is(err, ErrNothingToResume) {
		t.Fatalf("got %v, want ErrNothingToResume", err)
	}
}

func TestResumeRefusesAMachineThatHoldsNoRecord(t *testing.T) {
	if err := at(t, t.TempDir(), nil).resume(writeBundle(t, "acme", "1.4.0")); !errors.Is(err, ErrNothingToResume) {
		t.Fatalf("got %v, want ErrNothingToResume", err)
	}
}

func TestResumeRefusesAnotherBuildOfTheSameVersion(t *testing.T) {
	root := t.TempDir()
	if err := installWithABrokenImageLoad(t, root, writeBundle(t, "acme", "1.4.0")); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	if err := at(t, root, nil).resume(writeBundle(t, "acme", "1.4.0")); !errors.Is(err, ErrOtherBundle) {
		t.Fatalf("got %v, want ErrOtherBundle", err)
	}
}

func TestResumeRefusesAnActionThatMayHaveStoppedHalfway(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "one-action"))
	openAnAction(t, root, dir)
	shell := &countingShell{}
	job := at(t, root, nil).job(dir, nil)
	job.Shell = disk.NewShell(shell.run)
	err := job.Resume(context.Background(), io.Discard, report.New(io.Discard))
	if !errors.Is(err, ErrActionUnknown) {
		t.Fatalf("got %v, want ErrActionUnknown", err)
	}
	if len(shell.commands) != 0 {
		t.Errorf("the refusal ran %v", shell.commands)
	}
}

func TestResumeRefusesAnInstallAnOlderVesselLeftHalfway(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "one-action"))
	body, err := os.ReadFile("testdata/unfinished-main/acme/record.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	var entry release.Record
	if err := json.Unmarshal(body, &entry); err != nil {
		t.Fatalf("decode the fixture: %v", err)
	}
	entry.Root = at(t, root, nil).job(dir, nil).Artifact.Root
	body, err = json.Marshal(entry)
	if err != nil {
		t.Fatalf("encode the record: %v", err)
	}
	writeFile(t, filepath.Join(root, "var/lib/vessel/acme/record.json"), string(body))
	shell := &countingShell{}
	job := at(t, root, nil).job(dir, nil)
	job.Shell = disk.NewShell(shell.run)
	if err := job.Resume(context.Background(), io.Discard, report.New(io.Discard)); !errors.Is(err, ErrRecordTooOld) {
		t.Fatalf("got %v, want ErrRecordTooOld", err)
	}
	if len(shell.commands) != 0 {
		t.Errorf("the refusal ran %v", shell.commands)
	}
}

func TestResumeRefusesASetOnceTheValuesAreStored(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "one-value"))
	broken := []driver.Driver{brokenImages{quadlet.New(root, (&podmanStub{}).run)}}
	first := at(t, root, broken).job(dir, map[string]string{"PUBLIC_HOST": "a.acme.local"})
	if err := first.Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	second := at(t, root, nil).job(dir, map[string]string{"PUBLIC_HOST": "b.acme.local"})
	if err := second.Resume(context.Background(), io.Discard, report.New(io.Discard)); !errors.Is(err, ErrSetAfterValues) {
		t.Fatalf("got %v, want ErrSetAfterValues", err)
	}
}

func TestResumeFinishesAnInstallCutAtAnyStep(t *testing.T) {
	cuts := map[string]struct {
		kinds func(root string) []driver.Driver
		want  []release.Step
	}{
		"images": {func(root string) []driver.Driver {
			return []driver.Driver{brokenImages{quadlet.New(root, (&podmanStub{}).run)}}
		},
			[]release.Step{release.StepValues, release.StepActions, release.StepFiles, release.StepSecrets}},
		"services": {func(root string) []driver.Driver {
			return []driver.Driver{downServices{quadlet.New(root, (&podmanStub{}).run), []string{"web.service"}}}
		}, []release.Step{release.StepValues, release.StepActions, release.StepFiles, release.StepSecrets, release.StepImages}},
	}
	for name, cut := range cuts {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := writeBundle(t, "acme", "1.4.0")
			if err := at(t, root, cut.kinds(root)).install(dir); err == nil {
				t.Fatal("the install succeeded through the cut")
			}
			entry, err := recordsFor(t, root, "acme").Read()
			if err != nil {
				t.Fatalf("read the record: %v", err)
			}
			if !slices.Equal(entry.Steps, cut.want) {
				t.Fatalf("got %v, want %v before the resume", entry.Steps, cut.want)
			}
			if err := at(t, root, nil).resume(dir); err != nil {
				t.Fatalf("resume: %v", err)
			}
			entry, err = recordsFor(t, root, "acme").Read()
			if err != nil {
				t.Fatalf("read the record: %v", err)
			}
			if !entry.Done() {
				t.Error("the record is still open after the resume")
			}
		})
	}
}

func TestResumeFinishesAnInstallCutWritingAFile(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "etc/acme")
	writeFile(t, blocker, "an ordinary file where the delivery wants a directory")
	dir := blockedFileBundle(t)
	set := map[string]string{"PUBLIC_HOST": "dmas.acme.local"}
	if err := at(t, root, nil).job(dir, set).Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("the install succeeded with no file written")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatalf("remove the blocker: %v", err)
	}
	if err := at(t, root, nil).resume(dir); err != nil {
		t.Fatalf("resume: %v", err)
	}
}

func TestResumeAfterAFailedUpgradeRemovesTheStaleFile(t *testing.T) {
	root := t.TempDir()
	if err := at(t, root, nil).install(linkTestBundle(t, fixture(t, "stale-1.3"))); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	newer := linkTestBundle(t, fixture(t, "stale-1.4"))
	if err := at(t, root, nil).upgrade(newer); err == nil {
		t.Fatal("upgrade succeeded despite a shell that refuses every action")
	}
	job := at(t, root, nil).job(newer, nil)
	job.Shell = disk.NewShell(quietShell)
	if err := job.Resume(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/a.txt")); !os.IsNotExist(err) {
		t.Errorf("a.txt is still on disk after the resume, err=%v", err)
	}
}

type countingShell struct {
	commands []string
}

func (c *countingShell) run(_ context.Context, command string) ([]byte, error) {
	c.commands = append(c.commands, command)
	return nil, nil
}

func openAnAction(t *testing.T, root, dir string) {
	t.Helper()
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	records := recordsFor(t, root, "acme")
	entry, err := records.Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	entry.Steps = []release.Step{release.StepValues}
	entry.Actions = []release.Action{{Command: "mkdir -p /srv/acme"}}
	if err := records.Write(entry); err != nil {
		t.Fatalf("write the record: %v", err)
	}
}

func TestResumeCountsTheImagesTheDeliveryCarries(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	down := []driver.Driver{downServices{quadlet.New(root, (&podmanStub{}).run), []string{"web.service"}}}
	if err := at(t, root, down).install(dir); err == nil {
		t.Fatal("install succeeded with a service down")
	}
	var out bytes.Buffer
	if err := at(t, root, nil).job(dir, nil).Resume(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(out.String(), "1 images") {
		t.Errorf("got %q, want the one image the delivery carries", out.String())
	}
}

func TestResumeSaysTheFilesWereAlreadyWritten(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	var out bytes.Buffer
	if err := at(t, root, nil).job(dir, nil).Resume(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if !strings.Contains(out.String(), "files already written") {
		t.Errorf("got %q, want the files said already written", out.String())
	}
}

func TestSkipActionResumesWithoutRunningTheOpenAction(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "one-action"))
	openAnAction(t, root, dir)
	shell := &countingShell{}
	job := at(t, root, nil).job(dir, nil)
	job.Shell = disk.NewShell(shell.run)
	if err := job.SkipAction(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("skip the action: %v", err)
	}
	if len(shell.commands) != 0 {
		t.Errorf("got %v, want the open action left to the operator", shell.commands)
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !entry.Done() {
		t.Error("the record is still open after skipping the action")
	}
}

func TestSkipActionRefusesAMachineWithNoOpenAction(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	err := at(t, root, nil).job(dir, nil).SkipAction(context.Background(), io.Discard, report.New(io.Discard))
	if !errors.Is(err, ErrNoOpenAction) {
		t.Fatalf("got %v, want ErrNoOpenAction", err)
	}
}
