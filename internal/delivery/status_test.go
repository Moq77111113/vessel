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
	"time"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/site"
)

func TestStatusReportsTheVersionTheMachineHolds(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1757000000, 0).UTC(), End: time.Unix(1757000060, 0).UTC(),
	})
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "acme 1.4.0") {
		t.Errorf("got %q, want the name and version in it", out.String())
	}
}

func TestStatusSaysTheInstallNeverFinished(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet", Start: time.Unix(1, 0).UTC(),
	})
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "never finished") {
		t.Errorf("got %q, want it to name the unfinished install", out.String())
	}
}

func TestStatusNamesAFileThatDiffersSinceTheInstall(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc/containers/systemd/web.container")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []record.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:0000"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); !errors.Is(err, ErrRecordDoesNotMatch) {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "Differs") || !strings.Contains(out.String(), "web.container") {
		t.Errorf("got %q, want the file that differs named as differs", out.String())
	}
}

func TestStatusNamesAFileItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permissions cannot make a file unreadable")
	}
	root := t.TempDir()
	path := filepath.Join(root, "etc/containers/systemd/web.container")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Errorf("chmod: %v", err)
		}
	})
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []record.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:0000"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); !errors.Is(err, ErrRecordDoesNotMatch) {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "Unreadable") || !strings.Contains(out.String(), "web.container") {
		t.Errorf("got %q, want the unreadable file named as unreadable", out.String())
	}
	if strings.Contains(out.String(), "Differs") {
		t.Errorf("got %q, a file that cannot be read must not be reported as differing", out.String())
	}
}

func TestStatusNamesAFileAbsentSinceTheInstall(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []record.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:0000"}},
		Start: time.Unix(1, 0).UTC(),
	})
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); !errors.Is(err, ErrRecordDoesNotMatch) {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "Absent") || !strings.Contains(out.String(), "web.container") {
		t.Errorf("got %q, want the absent file named as absent", out.String())
	}
	if strings.Contains(out.String(), "Differs") {
		t.Errorf("got %q, an absent file must not be reported as differing", out.String())
	}
}

func TestStatusPrintsWhatTheMachineIsRunning(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	kind := downServices{quadlet.New((&podmanStub{}).run), []string{"web.service"}}
	var out bytes.Buffer
	if err := Status(context.Background(), &out, []machine.Machine{kind}, root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "web.service down") {
		t.Errorf("got %q, want the service named with the state it is in", out.String())
	}
}

func TestStatusFailsOnAKindThisBuildDoesNotKnow(t *testing.T) {
	err := Status(context.Background(), io.Discard, testKinds(), rootHoldingAComposeRecord(t), "acme")
	if !errors.Is(err, machine.ErrUnknownMachine) {
		t.Errorf("got %v, want ErrUnknownMachine", err)
	}
}

func TestStatusPrintsTheVersionBeforeItFails(t *testing.T) {
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), rootHoldingAComposeRecord(t), "acme"); err == nil {
		t.Fatal("status succeeded on a kind this build does not know")
	}
	if !strings.Contains(out.String(), "acme 1.4.0") {
		t.Errorf("got %q, want the version line printed before the failure", out.String())
	}
}

// rootHoldingAComposeRecord writes a record naming a machine kind this build cannot read.
func rootHoldingAComposeRecord(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "compose",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	return root
}

func TestStatusSaysTheMachineHoldsNoRecord(t *testing.T) {
	var out bytes.Buffer
	err := Status(context.Background(), &out, testKinds(), t.TempDir(), "acme")
	if !errors.Is(err, ErrNoRecord) {
		t.Errorf("got %v, want ErrNoRecord", err)
	}
}

func TestStatusWithNoNameSkipsADeliveryWithAnOddName(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	odd := filepath.Join(root, "var/lib/vessel", "Not-A-Name")
	if err := os.MkdirAll(odd, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(odd, "record.json"), []byte(`{"name":"Not-A-Name"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	var out bytes.Buffer
	if err := List(&out, root); err != nil {
		t.Fatalf("list: %v", err)
	}
	if want := "acme 1.4.0"; !strings.Contains(out.String(), want) {
		t.Errorf("got %q, want %q in it", out.String(), want)
	}
}

func testKinds() []machine.Machine {
	return []machine.Machine{quadlet.New((&podmanStub{}).run)}
}

func writeRecord(t *testing.T, root string, record record.Record) {
	t.Helper()
	if err := recordsFor(t, root, record.Name).Write(record); err != nil {
		t.Fatalf("write the record: %v", err)
	}
}

func recordsFor(t *testing.T, root, name string) *record.Records {
	t.Helper()
	dir, err := dirFor(root, name)
	if err != nil {
		t.Fatalf("dirFor: %v", err)
	}
	return record.NewRecords(dir)
}

func siteFor(t *testing.T, root, name string) *site.Store {
	t.Helper()
	dir, err := dirFor(root, name)
	if err != nil {
		t.Fatalf("dirFor: %v", err)
	}
	return site.NewStore(dir)
}

func TestStatusRefusesANameThatLeavesTheTargetRoot(t *testing.T) {
	err := Status(context.Background(), io.Discard, testKinds(), t.TempDir(), "../../../etc/cron.daily")
	if !errors.Is(err, descriptor.ErrDeliveryName) {
		t.Errorf("got %v, want ErrDeliveryName", err)
	}
}

func TestStatusPrintsTheVersionThisMachineCameFrom(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.3.0", Machine: "quadlet",
		Start: time.Unix(1756000000, 0).UTC(), End: time.Unix(1756000060, 0).UTC(),
	})
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1757000000, 0).UTC(), End: time.Unix(1757000060, 0).UTC(),
	})
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	want := "1.3.0, installed " + time.Unix(1756000000, 0).UTC().Format(time.RFC3339)
	if !strings.Contains(out.String(), want) {
		t.Errorf("got %q, want %q in it", out.String(), want)
	}
}

func TestStatusWithNoNameListsEveryDeliveryTheMachineHolds(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	writeRecord(t, root, record.Record{
		Name: "gateway", Version: "2.0.0", Machine: "quadlet",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := List(&out, root); err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, want := range []string{"acme 1.4.0", "gateway 2.0.0"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("got %q, want %q in it", out.String(), want)
		}
	}
}

func TestStatusFailsWhenAFileTheRecordNamesNoLongerMatches(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []record.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:0000"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	err := Status(context.Background(), io.Discard, testKinds(), root, "acme")
	if !errors.Is(err, ErrRecordDoesNotMatch) {
		t.Errorf("got %v, want ErrRecordDoesNotMatch", err)
	}
}

func TestStatusNamesTheVersionAnInstallPutOnTheMachine(t *testing.T) {
	root := t.TempDir()
	if err := runInstall(t, root, writeBundle(t, "acme", "1.4.0"), nil); err != nil {
		t.Fatalf("install: %v", err)
	}
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "acme 1.4.0") {
		t.Errorf("got %q, want the installed version named", out.String())
	}
}

func TestStatusSaysTheDeliveryIsInsecure(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet", Insecure: true,
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "insecure") {
		t.Errorf("got %q, want the delivery named insecure", out.String())
	}
}

func TestStatusNamesAnInsecureDeliveryThatNeverFinished(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, record.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet", Insecure: true, Start: time.Unix(1, 0).UTC(),
	})
	var out bytes.Buffer
	if err := Status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{"insecure", "never finished"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("got %q, want %q in it", out.String(), want)
		}
	}
}
