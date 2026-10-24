package build

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/report"
)

type orderRecorder struct{ log *[]string }

func (o *orderRecorder) Line(_, _ string) { *o.log = append(*o.log, "line") }

type recordingWriter struct{ log *[]string }

func (w *recordingWriter) Write(data []byte) (int, error) {
	*w.log = append(*w.log, "write")
	return len(data), nil
}

func stub() []byte { return []byte("#!/bin/sh\necho a real binary would be here\n") }

func bundleDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "images"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	for path, body := range map[string]string{
		"vessel.json":       `{"name":"acme"}`,
		"images/index.json": `{"schemaVersion":2}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, path), []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	return dir
}

func packed(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "acme-1.4.0")
	file, err := os.Create(out)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer file.Close()
	if err := Pack(bytes.NewReader(stub()), bundleDir(t), file, report.New(io.Discard)); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	return out
}

func TestPackPrintsEachFileAsItIsWritten(t *testing.T) {
	var progress bytes.Buffer
	if err := Pack(bytes.NewReader(stub()), bundleDir(t), io.Discard, report.New(&progress)); err != nil {
		t.Fatalf("Pack: %v", err)
	}
	for _, name := range []string{"vessel.json", "images/index.json"} {
		if !strings.Contains(progress.String(), name) {
			t.Errorf("progress does not name %s: %s", name, progress.String())
		}
	}
}

func TestPackAnnouncesEachFileBeforeWritingIt(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.json", "b.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	var log []string
	out := &recordingWriter{log: &log}
	work := &orderRecorder{log: &log}
	if err := writeTar(out, dir, work); err != nil {
		t.Fatalf("writeTar: %v", err)
	}
	if len(log) == 0 || log[0] != "line" {
		t.Fatalf("got %v, want the first event to be an announcement", log)
	}
	announcements := 0
	for i, event := range log {
		if event != "line" {
			continue
		}
		announcements++
		if i > 0 && log[i-1] == "line" {
			t.Errorf("two announcements in a row at %d, want a write between them: %v", i, log)
		}
	}
	if announcements != 2 {
		t.Errorf("got %d announcements, want 2, one per file: %v", announcements, log)
	}
}

func TestPackRefusesALink(t *testing.T) {
	dir := bundleDir(t)
	if err := os.Symlink("/etc/hostname", filepath.Join(dir, "linked")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	err := Pack(bytes.NewReader(stub()), dir, io.Discard, report.New(io.Discard))
	if !errors.Is(err, ErrNotAFile) {
		t.Fatalf("got %v, want ErrNotAFile", err)
	}
}

func TestPackKeepsTheStubRunnable(t *testing.T) {
	data, err := os.ReadFile(packed(t))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.HasPrefix(data, stub()) {
		t.Error("the stub is no longer at the front of the file")
	}
}
