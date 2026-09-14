package machine_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Moq77111113/vessel/internal/delivery"
	"github.com/Moq77111113/vessel/internal/machine"
)

func TestARecordSurvivesAWriteAndARead(t *testing.T) {
	root := t.TempDir()
	records := recordsFor(t, root, "acme")
	want := machine.Record{
		Name:    "acme",
		Version: "1.4.0",
		Machine: "quadlet",
		Root:    "sha256:aaaa",
		Files:   []machine.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:bbbb"}},
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
	_, found, err := recordsFor(t, t.TempDir(), "acme").Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if found {
		t.Error("found a record on an empty machine")
	}
}

func TestAWriteKeepsThePreviousRecord(t *testing.T) {
	records := recordsFor(t, t.TempDir(), "acme")
	if err := records.Write(machine.Record{Name: "acme", Version: "1.3.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(machine.Record{Name: "acme", Version: "1.4.0"}); err != nil {
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
	records := recordsFor(t, t.TempDir(), "acme")
	if err := records.Write(machine.Record{Name: "acme", Start: time.Now().UTC()}); err != nil {
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
	records := recordsFor(t, t.TempDir(), "acme")
	if err := records.Write(machine.Record{Name: "acme", Version: "1.3.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(machine.Record{Name: "acme", Version: "1.4.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Write(machine.Record{Name: "acme", Version: "1.4.0", End: time.Now().UTC()}); err != nil {
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

func TestRetireMovesTheRecordToPrevious(t *testing.T) {
	records := recordsFor(t, t.TempDir(), "acme")
	if err := records.Write(machine.Record{Name: "acme", Version: "1.4.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := records.Retire(); err != nil {
		t.Fatalf("retire: %v", err)
	}
	_, found, err := records.Read()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if found {
		t.Error("a record still reads back after Retire")
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
	if err := records.Write(machine.Record{Name: "acme", Version: "1.4.0"}); err == nil {
		t.Fatal("write overwrote a damaged record without complaint")
	}
	assertNoTempFile(t, root)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the damaged record disappeared: %v", err)
	}
}

func TestAFailedWriteLeavesTheDamagedRecordOnDisk(t *testing.T) {
	root := t.TempDir()
	records, path, damaged := damagedRecord(t, root)
	if err := records.Write(machine.Record{Name: "acme", Version: "1.4.0"}); err == nil {
		t.Fatal("write overwrote a damaged record without complaint")
	}
	assertNoTempFile(t, root)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != string(damaged) {
		t.Errorf("the damaged record was rewritten: %s", body)
	}
}

// damagedRecord writes a valid record, then replaces it on disk with bytes that are not JSON.
func damagedRecord(t *testing.T, root string) (records *machine.Records, path string, damaged []byte) {
	t.Helper()
	records = recordsFor(t, root, "acme")
	if err := records.Write(machine.Record{Name: "acme", Version: "1.3.0"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	path = filepath.Join(root, "var/lib/vessel", "acme", "record.json")
	damaged = []byte("not json")
	if err := os.WriteFile(path, damaged, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return records, path, damaged
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

func TestNewRecordsRefusesANameThatLeavesTheTargetRoot(t *testing.T) {
	_, err := machine.NewRecords(t.TempDir(), "../../../etc/cron.daily")
	if !errors.Is(err, delivery.ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func recordsFor(t *testing.T, root, name string) *machine.Records {
	t.Helper()
	records, err := machine.NewRecords(root, name)
	if err != nil {
		t.Fatalf("NewRecords: %v", err)
	}
	return records
}
