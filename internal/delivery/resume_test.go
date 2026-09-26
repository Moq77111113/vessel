package delivery

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

	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/report"
)

func TestResumeFinishesAnInstallCutAtTheImages(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	if err := runResume(t, root, dir, nil); err != nil {
		t.Fatalf("resume: %v", err)
	}
	entry, _, err := recordsFor(t, root, "acme").Read()
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
	first := jobFor(t, root, dir, []machine.Machine{brokenImages{quadlet.New((&podmanStub{}).run)}}, nil)
	first.Shell = machine.NewShell(shell.run)
	if err := first.Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	second := jobFor(t, root, dir, nil, nil)
	second.Shell = machine.NewShell(shell.run)
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
	if err := runInstall(t, root, dir, nil); err == nil {
		t.Fatal("the install succeeded with an action that failed")
	}
	shell := &countingShell{}
	job := jobFor(t, root, dir, nil, nil)
	job.Shell = machine.NewShell(shell.run)
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
	if err := runInstall(t, root, dir, nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := runResume(t, root, dir, nil); !errors.Is(err, ErrNothingToResume) {
		t.Fatalf("got %v, want ErrNothingToResume", err)
	}
}

func TestResumeRefusesAMachineThatHoldsNoRecord(t *testing.T) {
	if err := runResume(t, t.TempDir(), writeBundle(t, "acme", "1.4.0"), nil); !errors.Is(err, ErrNothingToResume) {
		t.Fatalf("got %v, want ErrNothingToResume", err)
	}
}

func TestResumeRefusesAnotherBuildOfTheSameVersion(t *testing.T) {
	root := t.TempDir()
	if err := installWithABrokenImageLoad(t, root, writeBundle(t, "acme", "1.4.0")); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	if err := runResume(t, root, writeBundle(t, "acme", "1.4.0"), nil); !errors.Is(err, ErrOtherBundle) {
		t.Fatalf("got %v, want ErrOtherBundle", err)
	}
}

func TestResumeRefusesAnActionThatMayHaveStoppedHalfway(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "one-action"))
	openAnAction(t, root, dir)
	shell := &countingShell{}
	job := jobFor(t, root, dir, nil, nil)
	job.Shell = machine.NewShell(shell.run)
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
	var entry record.Record
	if err := json.Unmarshal(body, &entry); err != nil {
		t.Fatalf("decode the fixture: %v", err)
	}
	entry.Root = jobFor(t, root, dir, nil, nil).Artifact.Root
	body, err = json.Marshal(entry)
	if err != nil {
		t.Fatalf("encode the record: %v", err)
	}
	writeFile(t, filepath.Join(root, "var/lib/vessel/acme/record.json"), string(body))
	shell := &countingShell{}
	job := jobFor(t, root, dir, nil, nil)
	job.Shell = machine.NewShell(shell.run)
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
	broken := []machine.Machine{brokenImages{quadlet.New((&podmanStub{}).run)}}
	first := jobFor(t, root, dir, broken, map[string]string{"PUBLIC_HOST": "a.acme.local"})
	if err := first.Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	second := jobFor(t, root, dir, nil, map[string]string{"PUBLIC_HOST": "b.acme.local"})
	if err := second.Resume(context.Background(), io.Discard, report.New(io.Discard)); !errors.Is(err, ErrSetAfterValues) {
		t.Fatalf("got %v, want ErrSetAfterValues", err)
	}
}

func TestResumeFinishesAnInstallCutAtAnyStep(t *testing.T) {
	cuts := map[string]struct {
		kinds func() []machine.Machine
		want  []record.Step
	}{
		"images": {func() []machine.Machine { return []machine.Machine{brokenImages{quadlet.New((&podmanStub{}).run)}} },
			[]record.Step{record.StepValues, record.StepActions, record.StepFiles, record.StepSecrets}},
		"services": {func() []machine.Machine {
			return []machine.Machine{downServices{quadlet.New((&podmanStub{}).run), []string{"web.service"}}}
		}, []record.Step{record.StepValues, record.StepActions, record.StepFiles, record.StepSecrets, record.StepImages}},
	}
	for name, cut := range cuts {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := writeBundle(t, "acme", "1.4.0")
			if err := runInstall(t, root, dir, cut.kinds()); err == nil {
				t.Fatal("the install succeeded through the cut")
			}
			entry, _, err := recordsFor(t, root, "acme").Read()
			if err != nil {
				t.Fatalf("read the record: %v", err)
			}
			if !slices.Equal(entry.Steps, cut.want) {
				t.Fatalf("got %v, want %v before the resume", entry.Steps, cut.want)
			}
			if err := runResume(t, root, dir, nil); err != nil {
				t.Fatalf("resume: %v", err)
			}
			entry, _, err = recordsFor(t, root, "acme").Read()
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
	if err := jobFor(t, root, dir, nil, set).Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("the install succeeded with no file written")
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatalf("remove the blocker: %v", err)
	}
	if err := runResume(t, root, dir, nil); err != nil {
		t.Fatalf("resume: %v", err)
	}
}

func TestResumeAfterAFailedUpgradeRemovesTheStaleFile(t *testing.T) {
	root := t.TempDir()
	if err := runInstall(t, root, linkTestBundle(t, fixture(t, "stale-1.3")), nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	newer := linkTestBundle(t, fixture(t, "stale-1.4"))
	if err := runUpgrade(t, root, newer, nil); err == nil {
		t.Fatal("upgrade succeeded despite a shell that refuses every action")
	}
	job := jobFor(t, root, newer, nil, nil)
	job.Shell = machine.NewShell(quietShell)
	if err := job.Resume(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc/acme/a.txt")); !os.IsNotExist(err) {
		t.Errorf("a.txt is still on disk after the resume, err=%v", err)
	}
}

// runResume resumes the bundle in dir under root, printing nothing.
func runResume(t *testing.T, root, dir string, kinds []machine.Machine) error {
	t.Helper()
	return jobFor(t, root, dir, kinds, nil).Resume(context.Background(), io.Discard, report.New(io.Discard))
}

// countingShell succeeds on every command and remembers each one.
type countingShell struct {
	commands []string
}

func (c *countingShell) run(_ context.Context, command string) ([]byte, error) {
	c.commands = append(c.commands, command)
	return nil, nil
}

// openAnAction leaves the record as a kill during the bundle's one action would.
func openAnAction(t *testing.T, root, dir string) {
	t.Helper()
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	records := recordsFor(t, root, "acme")
	entry, _, err := records.Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	entry.Steps = []record.Step{record.StepValues}
	entry.Actions = []record.Action{{Command: "mkdir -p /srv/acme"}}
	if err := records.Write(entry); err != nil {
		t.Fatalf("write the record: %v", err)
	}
}

func TestResumeCountsTheImagesTheDeliveryCarries(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	down := []machine.Machine{downServices{quadlet.New((&podmanStub{}).run), []string{"web.service"}}}
	if err := runInstall(t, root, dir, down); err == nil {
		t.Fatal("install succeeded with a service down")
	}
	var out bytes.Buffer
	if err := jobFor(t, root, dir, nil, nil).Resume(context.Background(), &out, report.New(io.Discard)); err != nil {
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
	if err := jobFor(t, root, dir, nil, nil).Resume(context.Background(), &out, report.New(io.Discard)); err != nil {
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
	job := jobFor(t, root, dir, nil, nil)
	job.Shell = machine.NewShell(shell.run)
	if err := job.SkipAction(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("skip the action: %v", err)
	}
	if len(shell.commands) != 0 {
		t.Errorf("got %v, want the open action left to the operator", shell.commands)
	}
	entry, _, err := recordsFor(t, root, "acme").Read()
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
	err := jobFor(t, root, dir, nil, nil).SkipAction(context.Background(), io.Discard, report.New(io.Discard))
	if !errors.Is(err, ErrNoOpenAction) {
		t.Fatalf("got %v, want ErrNoOpenAction", err)
	}
}
