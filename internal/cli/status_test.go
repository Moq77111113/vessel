package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
)

func TestStatusReportsTheVersionTheMachineHolds(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1757000000, 0).UTC(), End: time.Unix(1757000060, 0).UTC(),
	})
	var out bytes.Buffer
	if err := status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "acme 1.4.0") {
		t.Errorf("got %q, want the name and version in it", out.String())
	}
}

func TestStatusSaysTheInstallNeverFinished(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet", Start: time.Unix(1, 0).UTC(),
	})
	var out bytes.Buffer
	if err := status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "never finished") {
		t.Errorf("got %q, want it to name the unfinished install", out.String())
	}
}

func TestStatusNamesAFileThatChangedSinceTheInstall(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "etc/containers/systemd/web.container")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("edited by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []machine.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:0000"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "Changed") || !strings.Contains(out.String(), "web.container") {
		t.Errorf("got %q, want the changed file named as changed", out.String())
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
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []machine.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:0000"}},
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	if err := status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "Unreadable") || !strings.Contains(out.String(), "web.container") {
		t.Errorf("got %q, want the unreadable file named as unreadable", out.String())
	}
	if strings.Contains(out.String(), "Changed") {
		t.Errorf("got %q, a file that cannot be read must not be reported as changed", out.String())
	}
}

func TestStatusNamesAFileMissingSinceTheInstall(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Files: []machine.Entry{{Path: "etc/containers/systemd/web.container", Digest: "sha256:0000"}},
		Start: time.Unix(1, 0).UTC(),
	})
	var out bytes.Buffer
	if err := status(context.Background(), &out, testKinds(), root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "Missing") || !strings.Contains(out.String(), "web.container") {
		t.Errorf("got %q, want the missing file named as missing", out.String())
	}
	if strings.Contains(out.String(), "Changed") {
		t.Errorf("got %q, a missing file must not be reported as changed", out.String())
	}
}

func TestStatusPrintsWhatTheMachineIsRunning(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "quadlet",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	kind := downServices{quadlet.New((&podmanStub{}).run), []string{"web.service"}}
	var out bytes.Buffer
	if err := status(context.Background(), &out, []machine.Machine{kind}, root, "acme"); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "web.service") {
		t.Errorf("got %q, want the service named", out.String())
	}
}

func TestStatusReportsTheVersionThenFailsOnAKindThisBuildDoesNotKnow(t *testing.T) {
	root := t.TempDir()
	writeRecord(t, root, machine.Record{
		Name: "acme", Version: "1.4.0", Machine: "compose",
		Start: time.Unix(1, 0).UTC(), End: time.Unix(2, 0).UTC(),
	})
	var out bytes.Buffer
	err := status(context.Background(), &out, testKinds(), root, "acme")
	if !errors.Is(err, machine.ErrUnknownMachine) {
		t.Errorf("got %v, want ErrUnknownMachine", err)
	}
	if !strings.Contains(out.String(), "acme 1.4.0") {
		t.Errorf("got %q, want the version line printed before the failure", out.String())
	}
}

func TestStatusSaysTheMachineHoldsNoRecord(t *testing.T) {
	var out bytes.Buffer
	err := status(context.Background(), &out, testKinds(), t.TempDir(), "acme")
	if !errors.Is(err, ErrNoRecord) {
		t.Errorf("got %v, want ErrNoRecord", err)
	}
}

func testKinds() []machine.Machine {
	return []machine.Machine{quadlet.New((&podmanStub{}).run)}
}

func writeRecord(t *testing.T, root string, record machine.Record) {
	t.Helper()
	if err := machine.NewRecords(root, record.Name).Write(record); err != nil {
		t.Fatalf("write the record: %v", err)
	}
}
