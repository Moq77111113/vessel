package release_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Moq77111113/vessel/internal/site/release"
)

func TestARecordSurvivesAWriteAndARead(t *testing.T) {
	root := t.TempDir()
	records := recordsFor(t, filepath.Join(root, "acme"))
	want := release.Record{
		Name:    "acme",
		Version: "1.4.0",
		Machine: "quadlet",
		Root:    "sha256:aaaa",
		Files:   []release.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:bbbb"}},
		Images:  []string{"sha256:cccc"},
		Secrets: []string{"DB_PASSWORD"},
		Start:   time.Unix(1757000000, 0).UTC(),
	}
	if err := records.Write(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertNoTempFile(t, root)
	got, err := records.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Version != want.Version || len(got.Files) != 1 || got.Files[0].Digest != "sha256:bbbb" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadReportsNoRecordOnAMachineThatHoldsNone(t *testing.T) {
	_, err := recordsFor(t, t.TempDir()).Read()
	if !errors.Is(err, release.ErrNoRecord) {
		t.Errorf("got %v: found a record on an empty machine", err)
	}
}

func TestAWriteKeepsThePreviousRecord(t *testing.T) {
	records := recordsFor(t, t.TempDir())
	if err := records.Write(release.Record{Name: "acme", Version: "1.3.0", End: time.Unix(1757000000, 0).UTC()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(release.Record{Name: "acme", Version: "1.4.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	previous, err := records.Previous()
	if err != nil {
		t.Fatalf("previous: %v", err)
	}
	if previous.Version != "1.3.0" {
		t.Errorf("got %q, want %q", previous.Version, "1.3.0")
	}
}

func TestAnOpenRecordSaysTheInstallNeverFinished(t *testing.T) {
	records := recordsFor(t, t.TempDir())
	if err := records.Write(release.Record{Name: "acme", Start: time.Now().UTC()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := records.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Done() {
		t.Error("a record with no end time reports a finished install")
	}
}

func TestClosingARecordAtTheSameVersionKeepsThePreviousRecord(t *testing.T) {
	records := recordsFor(t, t.TempDir())
	if err := records.Write(release.Record{Name: "acme", Version: "1.3.0", End: time.Unix(1757000000, 0).UTC()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(release.Record{Name: "acme", Version: "1.4.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(release.Record{Name: "acme", Version: "1.4.0", End: time.Now().UTC()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	previous, err := records.Previous()
	if err != nil {
		t.Fatalf("previous: %v", err)
	}
	if previous.Version != "1.3.0" {
		t.Errorf("got %q, want %q", previous.Version, "1.3.0")
	}
}

func TestRemoveMovesTheRecordToPrevious(t *testing.T) {
	records := recordsFor(t, t.TempDir())
	if err := records.Write(release.Record{Name: "acme", Version: "1.4.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	_, err := records.Read()
	if !errors.Is(err, release.ErrNoRecord) {
		t.Errorf("got %v: a record still reads back after Remove", err)
	}
	previous, err := records.Previous()
	if err != nil {
		t.Fatalf("previous: %v", err)
	}
	if previous.Version != "1.4.0" {
		t.Errorf("got %q, want %q", previous.Version, "1.4.0")
	}
}

func TestWriteRefusesToOverwriteADamagedRecord(t *testing.T) {
	root := t.TempDir()
	fixture := junkRecord(t, root)
	records, path := fixture.records, fixture.path
	if err := records.Write(release.Record{Name: "acme", Version: "1.4.0"}); err == nil {
		t.Fatal("write overwrote a junk record without complaint")
	}
	assertNoTempFile(t, root)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the junk record disappeared: %v", err)
	}
}

func TestAFailedWriteLeavesTheDamagedRecordOnDisk(t *testing.T) {
	root := t.TempDir()
	fixture := junkRecord(t, root)
	records, path, junk := fixture.records, fixture.path, fixture.junk
	if err := records.Write(release.Record{Name: "acme", Version: "1.4.0"}); err == nil {
		t.Fatal("write overwrote a junk record without complaint")
	}
	assertNoTempFile(t, root)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != string(junk) {
		t.Errorf("the junk record was rewritten: %s", body)
	}
}

type damage struct {
	records *release.Records
	path    string
	junk    []byte
}

func junkRecord(t *testing.T, root string) damage {
	t.Helper()
	dir := filepath.Join(root, "acme")
	records := recordsFor(t, dir)
	if err := records.Write(release.Record{Name: "acme", Version: "1.3.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	path := filepath.Join(dir, "record.json")
	junk := []byte("not json")
	if err := os.WriteFile(path, junk, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return damage{records: records, path: path, junk: junk}
}

func assertNoTempFile(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".vessel-") {
			t.Errorf("leftover temp file %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func recordsFor(t *testing.T, dir string) *release.Records {
	t.Helper()
	return release.NewRecords(dir)
}

func TestARecordWithNoInsecureKeyReadsAsSigned(t *testing.T) {
	entry, err := release.NewRecords("testdata/older").Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if entry.Insecure {
		t.Error("a record an older vessel wrote reads as insecure")
	}
}

func TestARecordHasAStepItLists(t *testing.T) {
	entry := release.Record{Steps: []release.Step{release.StepValues, release.StepActions}}
	if !entry.Has(release.StepActions) {
		t.Error("the record does not have a step it lists")
	}
	if entry.Has(release.StepFiles) {
		t.Error("the record has a step it does not list")
	}
}

func TestAnActionWithNoEndIsOpen(t *testing.T) {
	entry := release.Record{Actions: []release.Action{{Command: "a", End: time.Unix(1, 0)}, {Command: "b"}}}
	action, open := entry.OpenAction()
	if !open || action.Command != "b" {
		t.Errorf("got %+v open=%v, want b open", action, open)
	}
}

func TestARecordWhoseActionsAllEndedHasNoOpenAction(t *testing.T) {
	entry := release.Record{Actions: []release.Action{{Command: "a", End: time.Unix(1, 0)}}}
	if _, open := entry.OpenAction(); open {
		t.Error("an action that ended reads as open")
	}
	if _, open := (release.Record{}).OpenAction(); open {
		t.Error("a record with no action has an open one")
	}
}

func TestARecordWithNoStepsStillReadsAsDone(t *testing.T) {
	entry, err := release.NewRecords("testdata/main").Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !entry.Done() {
		t.Error("a finished record an older vessel wrote reads as unfinished")
	}
	if _, open := entry.OpenAction(); open {
		t.Error("a record with no actions key has an open action")
	}
}

func TestAnOpenRecordNeverBecomesThePreviousOne(t *testing.T) {
	records := recordsFor(t, filepath.Join(t.TempDir(), "acme"))
	finished := time.Unix(1757000000, 0).UTC()
	for _, entry := range []release.Record{
		{Name: "acme", Version: "1.3.0", End: finished},
		{Name: "acme", Version: "1.4.0"},
		{Name: "acme", Version: "1.3.0"},
	} {
		if err := records.Write(entry); err != nil {
			t.Fatalf("write %s: %v", entry.Version, err)
		}
	}
	previous, err := records.Previous()
	if err != nil {
		t.Fatalf("read the previous record: %v", err)
	}
	if previous.Version != "1.3.0" || !previous.Done() {
		t.Errorf("got %s done=%v, want 1.3.0 done", previous.Version, previous.Done())
	}
}
