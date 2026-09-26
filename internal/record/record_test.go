package record_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Moq77111113/vessel/internal/record"
)

func TestARecordSurvivesAWriteAndARead(t *testing.T) {
	root := t.TempDir()
	records := recordsFor(t, filepath.Join(root, "acme"))
	want := record.Record{
		Name:    "acme",
		Version: "1.4.0",
		Machine: "quadlet",
		Root:    "sha256:aaaa",
		Files:   []record.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:bbbb"}},
		Images:  []string{"sha256:cccc"},
		Secrets: []string{"DB_PASSWORD"},
		Start:   time.Unix(1757000000, 0).UTC(),
	}
	if err := records.Write(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	assertNoTempFile(t, root)
	got, found, err := records.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !found {
		t.Fatal("no record on a machine that was just written to")
	}
	if got.Version != want.Version || len(got.Files) != 1 || got.Files[0].Digest != "sha256:bbbb" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestReadReportsNoRecordOnAMachineThatHoldsNone(t *testing.T) {
	_, found, err := recordsFor(t, t.TempDir()).Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if found {
		t.Error("found a record on an empty machine")
	}
}

func TestAWriteKeepsThePreviousRecord(t *testing.T) {
	records := recordsFor(t, t.TempDir())
	if err := records.Write(record.Record{Name: "acme", Version: "1.3.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(record.Record{Name: "acme", Version: "1.4.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	previous, found, err := records.Previous()
	if err != nil {
		t.Fatalf("previous: %v", err)
	}
	if !found || previous.Version != "1.3.0" {
		t.Errorf("got %q, want %q", previous.Version, "1.3.0")
	}
}

func TestAnOpenRecordSaysTheInstallNeverFinished(t *testing.T) {
	records := recordsFor(t, t.TempDir())
	if err := records.Write(record.Record{Name: "acme", Start: time.Now().UTC()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, _, err := records.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Done() {
		t.Error("a record with no end time reports a finished install")
	}
}

func TestClosingARecordAtTheSameVersionKeepsThePreviousRecord(t *testing.T) {
	records := recordsFor(t, t.TempDir())
	if err := records.Write(record.Record{Name: "acme", Version: "1.3.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(record.Record{Name: "acme", Version: "1.4.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(record.Record{Name: "acme", Version: "1.4.0", End: time.Now().UTC()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	previous, found, err := records.Previous()
	if err != nil {
		t.Fatalf("previous: %v", err)
	}
	if !found || previous.Version != "1.3.0" {
		t.Errorf("got %q, want %q", previous.Version, "1.3.0")
	}
}

func TestRemoveMovesTheRecordToPrevious(t *testing.T) {
	records := recordsFor(t, t.TempDir())
	if err := records.Write(record.Record{Name: "acme", Version: "1.4.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Remove(); err != nil {
		t.Fatalf("remove: %v", err)
	}
	_, found, err := records.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if found {
		t.Error("a record still reads back after Remove")
	}
	previous, found, err := records.Previous()
	if err != nil {
		t.Fatalf("previous: %v", err)
	}
	if !found || previous.Version != "1.4.0" {
		t.Errorf("got %q, want %q", previous.Version, "1.4.0")
	}
}

func TestWriteRefusesToOverwriteADamagedRecord(t *testing.T) {
	root := t.TempDir()
	records, path, _ := damagedRecord(t, root)
	if err := records.Write(record.Record{Name: "acme", Version: "1.4.0"}); err == nil {
		t.Fatal("write overwrote a junk record without complaint")
	}
	assertNoTempFile(t, root)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the junk record disappeared: %v", err)
	}
}

func TestAFailedWriteLeavesTheDamagedRecordOnDisk(t *testing.T) {
	root := t.TempDir()
	records, path, junk := damagedRecord(t, root)
	if err := records.Write(record.Record{Name: "acme", Version: "1.4.0"}); err == nil {
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

// damagedRecord writes a valid record, then replaces it on disk with bytes that are not JSON.
func damagedRecord(t *testing.T, root string) (records *record.Records, path string, junk []byte) {
	t.Helper()
	dir := filepath.Join(root, "acme")
	records = recordsFor(t, dir)
	if err := records.Write(record.Record{Name: "acme", Version: "1.3.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	path = filepath.Join(dir, "record.json")
	junk = []byte("not json")
	if err := os.WriteFile(path, junk, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return records, path, junk
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

func recordsFor(t *testing.T, dir string) *record.Records {
	t.Helper()
	return record.NewRecords(dir)
}

func TestARecordWithNoInsecureKeyReadsAsSigned(t *testing.T) {
	entry, found, err := record.NewRecords("testdata/older").Read()
	if err != nil || !found {
		t.Fatalf("read: found=%v err=%v", found, err)
	}
	if entry.Insecure {
		t.Error("a record an older vessel wrote reads as insecure")
	}
}
