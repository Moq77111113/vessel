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
	"time"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/pkg/driver"
)

type afterShell struct {
	commands []string
	fail     string
	kill     string
}

func (a *afterShell) run(ctx context.Context, command string, stdout, stderr io.Writer) error {
	a.commands = append(a.commands, command)
	switch command {
	case a.fail:
		io.WriteString(stdout, "out of luck\n")
		io.WriteString(stderr, "no luck\n")
		return errors.New("exit status 1")
	case a.kill:
		return disk.Sh(ctx, "kill -9 $$", stdout, stderr)
	}
	io.WriteString(stdout, "said "+command+"\n")
	io.WriteString(stderr, "warned "+command+"\n")
	return nil
}

func afterJob(t *testing.T, root, dir string, shell *afterShell, kinds []driver.Driver) testJob {
	t.Helper()
	job := at(t, root, kinds).job(dir, nil)
	job.Shell = disk.NewShell(shell.run)
	return job
}

func TestInstallShowsBothStreamsOfAnAfterAction(t *testing.T) {
	var out bytes.Buffer
	job := afterJob(t, t.TempDir(), linkTestBundle(t, fixture(t, "after-actions")), &afterShell{}, nil)
	if err := job.Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), "said echo ready\n") || !strings.Contains(out.String(), "warned echo ready\n") {
		t.Errorf("got %q, want stdout and stderr of echo ready", out.String())
	}
}

func TestInstallShowsNothingOfAMutedAfterAction(t *testing.T) {
	var out bytes.Buffer
	shell := &afterShell{}
	job := afterJob(t, t.TempDir(), linkTestBundle(t, fixture(t, "after-actions")), shell, nil)
	if err := job.Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !slices.Contains(shell.commands, "echo quiet") {
		t.Fatalf("got %v, want the muted action run", shell.commands)
	}
	if strings.Contains(out.String(), "echo quiet") {
		t.Errorf("got %q, want the muted output hidden", out.String())
	}
}

func TestInstallShowsNothingOfAnActionThatSucceeds(t *testing.T) {
	var out bytes.Buffer
	job := afterJob(t, t.TempDir(), linkTestBundle(t, fixture(t, "after-actions")), &afterShell{}, nil)
	if err := job.Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	if strings.Contains(out.String(), "mkdir") {
		t.Errorf("got %q, want the action output hidden", out.String())
	}
}

func TestInstallRunsTheAfterActionsInOrderAfterTheActions(t *testing.T) {
	shell := &afterShell{}
	job := afterJob(t, t.TempDir(), linkTestBundle(t, fixture(t, "after-actions")), shell, nil)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	want := []string{"mkdir -p /srv/acme", "touch /srv/acme/ready", "echo ready", "echo quiet"}
	if !slices.Equal(shell.commands, want) {
		t.Errorf("got %v, want %v", shell.commands, want)
	}
}

func TestEachRecordedActionNamesItsStep(t *testing.T) {
	root := t.TempDir()
	job := afterJob(t, root, linkTestBundle(t, fixture(t, "after-actions")), &afterShell{}, nil)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("install: %v", err)
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	var steps []release.Step
	for _, action := range entry.Actions {
		steps = append(steps, action.Step)
	}
	want := []release.Step{release.StepActions, release.StepActions, release.StepAfter, release.StepAfter}
	if !slices.Equal(steps, want) {
		t.Errorf("got %v, want %v", steps, want)
	}
}

func TestInstallRunsNoAfterActionWhenAServiceIsDown(t *testing.T) {
	root := t.TempDir()
	shell := &afterShell{}
	down := []driver.Driver{downServices{quadlet.New(root, (&podmanStub{}).run), []string{"web.service"}}}
	job := afterJob(t, root, linkTestBundle(t, fixture(t, "after-actions")), shell, down)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); !errors.Is(err, ErrServiceIsDown) {
		t.Fatalf("got %v, want ErrServiceIsDown", err)
	}
	if want := []string{"mkdir -p /srv/acme", "touch /srv/acme/ready"}; !slices.Equal(shell.commands, want) {
		t.Errorf("got %v, want no after action run", shell.commands)
	}
}

func TestAFailingAfterActionStopsTheInstallWithTheRecordOpen(t *testing.T) {
	root := t.TempDir()
	shell := &afterShell{fail: "echo ready"}
	job := afterJob(t, root, linkTestBundle(t, fixture(t, "after-actions")), shell, nil)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); !errors.Is(err, disk.ErrAction) {
		t.Fatalf("got %v, want disk.ErrAction", err)
	}
	if slices.Contains(shell.commands, "echo quiet") {
		t.Errorf("got %v, want nothing run after the failure", shell.commands)
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if entry.Done() || entry.Has(release.StepAfter) || !entry.Has(release.StepServices) {
		t.Errorf("got steps %v done %v, want services finished and the record open", entry.Steps, entry.Done())
	}
}

func TestAFailingMutedAfterActionReportsOnlyItsCommandAndExit(t *testing.T) {
	shell := &afterShell{fail: "echo quiet"}
	job := afterJob(t, t.TempDir(), linkTestBundle(t, fixture(t, "after-actions")), shell, nil)
	err := job.Run(context.Background(), io.Discard, report.New(io.Discard))
	if !errors.Is(err, disk.ErrAction) {
		t.Fatalf("got %v, want disk.ErrAction", err)
	}
	for _, want := range []string{"echo quiet", "exit status 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got %q, want %q in it", err.Error(), want)
		}
	}
}

func TestAFailingMutedAfterActionKeepsItsOutputOffTheErrorAndTheScreen(t *testing.T) {
	var out, log bytes.Buffer
	shell := &afterShell{fail: "echo quiet"}
	job := afterJob(t, t.TempDir(), linkTestBundle(t, fixture(t, "after-actions")), shell, nil)
	err := job.Run(context.Background(), &out, report.New(&log))
	if err == nil {
		t.Fatal("the install succeeded with a muted action that failed")
	}
	for _, secret := range []string{"out of luck", "no luck"} {
		if strings.Contains(err.Error(), secret) || strings.Contains(out.String(), secret) || strings.Contains(log.String(), secret) {
			t.Errorf("got error %q, screen %q, log %q, want %q in none", err.Error(), out.String(), log.String(), secret)
		}
	}
}

func TestAFailingActionReportsItsExitAndBothStreams(t *testing.T) {
	shell := &afterShell{fail: "touch /srv/acme/ready"}
	job := afterJob(t, t.TempDir(), linkTestBundle(t, fixture(t, "after-actions")), shell, nil)
	err := job.Run(context.Background(), io.Discard, report.New(io.Discard))
	if !errors.Is(err, disk.ErrAction) {
		t.Fatalf("got %v, want disk.ErrAction", err)
	}
	for _, want := range []string{"touch /srv/acme/ready", "exit status 1", "out of luck", "no luck"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got %q, want %q in it", err.Error(), want)
		}
	}
}

func TestResumeRunsOnlyTheActionsTheCutInstallLeft(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "after-actions"))
	if err := afterJob(t, root, dir, &afterShell{fail: "touch /srv/acme/ready"}, nil).Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("the install succeeded with an action that failed")
	}
	shell := &afterShell{}
	if err := afterJob(t, root, dir, shell, nil).Resume(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if want := []string{"touch /srv/acme/ready", "echo ready", "echo quiet"}; !slices.Equal(shell.commands, want) {
		t.Errorf("got %v, want %v", shell.commands, want)
	}
}

func TestResumeRunsOnlyTheAfterActionsTheCutInstallLeft(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "after-actions"))
	if err := afterJob(t, root, dir, &afterShell{fail: "echo quiet"}, nil).Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("the install succeeded with an after action that failed")
	}
	shell := &afterShell{}
	if err := afterJob(t, root, dir, shell, nil).Resume(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if want := []string{"echo quiet"}; !slices.Equal(shell.commands, want) {
		t.Errorf("got %v, want %v", shell.commands, want)
	}
	entry, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if !entry.Done() {
		t.Error("the record is still open after a resume that succeeded")
	}
}

func TestAFailedWriteToTheOperatorNeverRunsTheAfterActionAgain(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "after-actions"))
	if err := afterJob(t, root, dir, &afterShell{}, nil).Run(context.Background(), brokenOut{}, report.New(io.Discard)); !errors.Is(err, disk.ErrOutput) {
		t.Fatalf("got %v, want disk.ErrOutput", err)
	}
	shell := &afterShell{}
	if err := afterJob(t, root, dir, shell, nil).Resume(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if want := []string{"echo quiet"}; !slices.Equal(shell.commands, want) {
		t.Errorf("got %v, want echo ready left alone", shell.commands)
	}
}

func TestResumeRefusesAnAfterActionKilledHalfwayUntilSkipped(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "after-actions"))
	if err := afterJob(t, root, dir, &afterShell{kill: "echo ready"}, nil).Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("the install succeeded with an after action killed by a signal")
	}
	shell := &afterShell{}
	if err := afterJob(t, root, dir, shell, nil).Resume(context.Background(), io.Discard, report.New(io.Discard)); !errors.Is(err, ErrActionUnknown) {
		t.Fatalf("got %v, want ErrActionUnknown", err)
	}
	if err := afterJob(t, root, dir, shell, nil).SkipAction(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("skip the action: %v", err)
	}
	if want := []string{"echo quiet"}; !slices.Equal(shell.commands, want) {
		t.Errorf("got %v, want only the action after the skipped one", shell.commands)
	}
}

func TestADryRunNamesTheAfterActionsOnceTheServicesStart(t *testing.T) {
	var out bytes.Buffer
	job := at(t, t.TempDir(), nil).job(linkTestBundle(t, fixture(t, "after-actions")), nil)
	if err := job.Preview().Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	output := collapse(out.String())
	start, ready, quiet := strings.Index(output, "Start web.service"), strings.Index(output, "Run echo ready"), strings.Index(output, "Run echo quiet (mute)")
	if start < 0 || ready < start || quiet < ready {
		t.Errorf("got %q, want both after actions run in order once the services start, the muted one marked", output)
	}
}

func TestADryRunOfAResumeNamesOnlyTheAfterActionsLeft(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "after-actions"))
	if err := afterJob(t, root, dir, &afterShell{fail: "echo quiet"}, nil).Run(context.Background(), io.Discard, report.New(io.Discard)); err == nil {
		t.Fatal("the install succeeded with an after action that failed")
	}
	var out bytes.Buffer
	if err := afterJob(t, root, dir, &afterShell{}, nil).Preview().Resume(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	output := collapse(out.String())
	if strings.Contains(output, "Run echo ready") || strings.Contains(output, "Run touch") || !strings.Contains(output, "Run echo quiet (mute)") {
		t.Errorf("got %q, want only echo quiet left to run", output)
	}
}

func TestResumeRefusesARecordWhoseActionsNameNoStep(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "after-actions"))
	body, err := os.ReadFile("testdata/format-2/acme/record.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	var entry release.Record
	if err := json.Unmarshal(body, &entry); err != nil {
		t.Fatalf("decode the fixture: %v", err)
	}
	entry.Root = at(t, root, nil).job(dir, nil).Artifact.Root
	entry.End = time.Time{}
	body, err = json.Marshal(entry)
	if err != nil {
		t.Fatalf("encode the record: %v", err)
	}
	writeFile(t, filepath.Join(root, "var/lib/vessel/acme/record.json"), string(body))
	shell := &afterShell{}
	err = afterJob(t, root, dir, shell, nil).Resume(context.Background(), io.Discard, report.New(io.Discard))
	if !errors.Is(err, ErrRecordTooOld) {
		t.Fatalf("got %v, want ErrRecordTooOld", err)
	}
	if errors.Is(err, ErrPartlyInstalled) || len(shell.commands) != 0 {
		t.Errorf("the refusal ran %v or said the machine changed", shell.commands)
	}
}

func TestUpgradeRefusesARecordWhoseActionsNameNoStep(t *testing.T) {
	root := t.TempDir()
	body, err := os.ReadFile("testdata/format-2/acme/record.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	writeFile(t, filepath.Join(root, "var/lib/vessel/acme/record.json"), string(body))
	err = at(t, root, nil).upgrade(writeBundle(t, "acme", "1.5.0"))
	if !errors.Is(err, ErrRecordTooOld) {
		t.Fatalf("got %v, want ErrRecordTooOld", err)
	}
	if errors.Is(err, ErrPartlyInstalled) {
		t.Error("a refusal before any change says the machine changed")
	}
}

func TestInstallRefusesARecordWhoseActionsNameNoStep(t *testing.T) {
	root := t.TempDir()
	body, err := os.ReadFile("testdata/format-2/acme/record.json")
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	writeFile(t, filepath.Join(root, "var/lib/vessel/acme/record.json"), string(body))
	shell := &afterShell{}
	err = afterJob(t, root, linkTestBundle(t, fixture(t, "after-actions")), shell, nil).Run(context.Background(), io.Discard, report.New(io.Discard))
	if !errors.Is(err, ErrRecordTooOld) {
		t.Fatalf("got %v, want ErrRecordTooOld", err)
	}
	if errors.Is(err, ErrPartlyInstalled) || len(shell.commands) != 0 {
		t.Errorf("the refusal ran %v or said the machine changed", shell.commands)
	}
}

type brokenOut struct{}

func (brokenOut) Write([]byte) (int, error) { return 0, errors.New("terminal gone") }
