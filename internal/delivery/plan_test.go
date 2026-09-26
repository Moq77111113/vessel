package delivery

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/report"
)

func TestADryRunChangesNothingOnTheMachine(t *testing.T) {
	root := t.TempDir()
	stub := &podmanStub{}
	job := jobFor(t, root, dryRunBundle(t), []machine.Machine{quadlet.New(stub.run)}, nil)
	job.SetFile = dbPassword
	if err := job.Preview().Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %v under the root, want nothing", entries)
	}
	for _, call := range stub.calls {
		if !readOnly(call) {
			t.Errorf("the dry run ran %q", call)
		}
	}
}

func TestADryRunPrintsEveryChange(t *testing.T) {
	output := dryRun(t, t.TempDir(), dryRunBundle(t))
	for _, want := range []string{
		"dry run",
		"Add etc/containers/systemd/web.container",
		"Add etc/acme/realm.json, content depends on PUBLIC_HOST",
		"Create secret DB_PASSWORD",
		"Run mkdir -p /srv/acme",
		"Start web.service",
		"Unverified PUBLIC_HOST from hostname -f, not run",
		"Unverified the result of every action",
		"Unverified service health, known once started",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("got %q, want %q in it", output, want)
		}
	}
}

func TestADryRunNeverPrintsASecretValue(t *testing.T) {
	if output := dryRun(t, t.TempDir(), dryRunBundle(t)); strings.Contains(output, "hunter2") {
		t.Errorf("got %q, want no secret value in it", output)
	}
}

func TestADryRunOfAnUpgradeNamesWhatItReplacesAndRemoves(t *testing.T) {
	root := t.TempDir()
	if err := runInstall(t, root, linkTestBundle(t, fixture(t, "acme-1.3")), nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	var out bytes.Buffer
	job := jobFor(t, root, linkTestBundle(t, fixture(t, "acme-1.4")), nil, nil)
	if err := job.Preview().Upgrade(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	output := collapse(out.String())
	for _, want := range []string{"Remove etc/acme/a.txt", "Replace etc/acme/b.txt", "Add etc/acme/c.txt"} {
		if !strings.Contains(output, want) {
			t.Errorf("got %q, want %q in it", output, want)
		}
	}
	entry, _, err := recordsFor(t, root, "acme").Read()
	if err != nil {
		t.Fatalf("read the record: %v", err)
	}
	if entry.Version != "1.3.0" {
		t.Errorf("got %s, want the 1.3.0 record untouched", entry.Version)
	}
	body, err := os.ReadFile(filepath.Join(root, "etc/acme/b.txt"))
	if err != nil || string(body) != "b from 1.3\n" {
		t.Errorf("got %q (%v), want b.txt from 1.3.0 untouched", body, err)
	}
	if _, found, _ := recordsFor(t, root, "acme").Previous(); found {
		t.Error("the dry run rotated the record")
	}
}

func TestADryRunOfAResumeSkipsTheStepsTheRecordHas(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	var out bytes.Buffer
	if err := jobFor(t, root, dir, nil, nil).Preview().Resume(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	output := collapse(out.String())
	if !strings.Contains(output, "Skip files, the last run finished it") {
		t.Errorf("got %q, want the files step skipped", output)
	}
	if strings.Contains(output, "Add etc/") || strings.Contains(output, "Keep etc/") {
		t.Errorf("got %q, want no file line for a step the record has", output)
	}
}

func TestADryRunStillRefusesAnUnfinishedInstall(t *testing.T) {
	root := t.TempDir()
	dir := writeBundle(t, "acme", "1.4.0")
	if err := installWithABrokenImageLoad(t, root, dir); err == nil {
		t.Fatal("install succeeded with a broken image load")
	}
	err := jobFor(t, root, writeBundle(t, "acme", "1.5.0"), nil, nil).Preview().Run(context.Background(), io.Discard, report.New(io.Discard))
	if !errors.Is(err, ErrRecordOpen) {
		t.Fatalf("got %v, want ErrRecordOpen", err)
	}
}

// readOnly reports whether a podman or systemctl call only reads the machine.
func readOnly(call string) bool {
	return call == "podman --version" || call == "podman image ls" || call == "podman secret ls --format {{.Name}}" ||
		strings.HasPrefix(call, "systemctl is-active ")
}

// dryRunBundle links testdata/dry-run with a unit that reads DB_PASSWORD.
// dbPassword answers the secret the dry-run fixture declares, as --set-file would.
var dbPassword = map[string]string{"DB_PASSWORD": "hunter2"}

func dryRunBundle(t *testing.T) string {
	t.Helper()
	return linkTestDelivery(t, "Secret=DB_PASSWORD,type=env,target=DB\n", fixture(t, "dry-run"))
}

// dryRun previews an install of the dry-run fixture under root, and returns what it printed.
func dryRun(t *testing.T, root, dir string) string {
	t.Helper()
	var out bytes.Buffer
	job := jobFor(t, root, dir, nil, nil)
	job.SetFile = dbPassword
	if err := job.Preview().Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	return collapse(out.String())
}

// collapse turns every run of blanks into one space, so a test reads a line without the verb column.
func collapse(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func TestADryRunOfAnUpgradeNamesTheServiceItStops(t *testing.T) {
	root := t.TempDir()
	if err := runInstall(t, root, linkTestBundle(t, fixture(t, "unit-1.3")), nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	var out bytes.Buffer
	if err := jobFor(t, root, linkTestBundle(t, fixture(t, "unit-1.4")), nil, nil).Preview().Upgrade(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if output := collapse(out.String()); !strings.Contains(output, "Stop cache.service") {
		t.Errorf("got %q, want the service of the dropped unit stopped", output)
	}
}

func TestADryRunNamesTheTimerItStarts(t *testing.T) {
	var out bytes.Buffer
	if err := jobFor(t, t.TempDir(), linkTestBundle(t, fixture(t, "timer")), nil, nil).Preview().Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if output := collapse(out.String()); !strings.Contains(output, "Start backup.timer") {
		t.Errorf("got %q, want the timer started", output)
	}
}

func TestADryRunNamesTheChecksItConfirmed(t *testing.T) {
	output := dryRun(t, t.TempDir(), dryRunBundle(t))
	for _, want := range []string{
		"Checked the machine is ready",
		"Checked the bundle matches its digests",
		"Checked every variable has a value",
		"Checked no install is left unfinished",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("got %q, want %q in it", output, want)
		}
	}
}

func TestADryRunNamesTheValueAFileOnDiskDependsOn(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "etc/acme/realm.json"), "{}")
	if output := dryRun(t, root, dryRunBundle(t)); !strings.Contains(output, "Depends etc/acme/realm.json on PUBLIC_HOST") {
		t.Errorf("got %q, want the file named with the value it depends on", output)
	}
}

func TestADryRunRollbackOpensWithTheHeaderAndMarksTheOpenAction(t *testing.T) {
	root := t.TempDir()
	older := linkTestBundle(t, fixture(t, "acme-1.3"))
	if err := runInstall(t, root, older, nil); err != nil {
		t.Fatalf("install 1.3.0: %v", err)
	}
	openAnAction(t, root, linkTestBundle(t, fixture(t, "one-action")))
	var out bytes.Buffer
	if err := jobFor(t, root, older, nil, nil).Preview().Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if !strings.HasPrefix(out.String(), "acme 1.3.0, dry run") {
		t.Errorf("got %q, want the dry run header first", out.String())
	}
	if !strings.Contains(collapse(out.String()), "Unverified mkdir -p /srv/acme may have stopped halfway, this rollback passes over it") {
		t.Errorf("got %q, want the open action named as unverified", out.String())
	}
}

func TestADryRunOfSkipActionNamesTheActionItMarksFinished(t *testing.T) {
	root := t.TempDir()
	dir := linkTestBundle(t, fixture(t, "one-action"))
	openAnAction(t, root, dir)
	var out bytes.Buffer
	if err := jobFor(t, root, dir, nil, nil).Preview().SkipAction(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if output := collapse(out.String()); !strings.Contains(output, "Skip mkdir -p /srv/acme, marked finished without running it") {
		t.Errorf("got %q, want the skipped action named", output)
	}
}

func TestADryRunWithNoServiceSaysNothingAboutHealth(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	contents := bundle.Contents{Layout: emptyLayout(t), Config: bundle.Config{Name: "acme", Version: "1.4.0", Machine: "quadlet", Platform: "linux/amd64"}}
	if err := bundle.Write(dir, contents); err != nil {
		t.Fatalf("write the bundle: %v", err)
	}
	var out bytes.Buffer
	if err := jobFor(t, t.TempDir(), dir, nil, nil).Preview().Run(context.Background(), &out, report.New(io.Discard)); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if strings.Contains(out.String(), "service health") {
		t.Errorf("got %q, want no health line without a service", out.String())
	}
}

func TestADryRunRunsNoFromEvenWithAShellSetAfterPreview(t *testing.T) {
	job := jobFor(t, t.TempDir(), dryRunBundle(t), nil, nil).Preview()
	job.SetFile = dbPassword
	job.Shell = machine.NewShell(noCapture)
	if err := job.Run(context.Background(), io.Discard, report.New(io.Discard)); err != nil {
		t.Fatalf("the dry run ran a from: command: %v", err)
	}
}
