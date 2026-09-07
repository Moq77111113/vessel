package report

import (
	"strings"
	"testing"
)

func TestLineRightAlignsTheVerbThenTheSubject(t *testing.T) {
	var out strings.Builder
	New(&out).Line("Resolving", "docker.io/library/postgres:18-alpine")
	if got, want := out.String(), "  Resolving docker.io/library/postgres:18-alpine\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLineStartsTheSubjectAtTheSameColumnForAShortAndALongVerb(t *testing.T) {
	var out strings.Builder
	report := New(&out)
	report.Line("Writing", "x")
	report.Line("Resolving", "y")
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), out.String())
	}
	firstAt := strings.IndexByte(lines[0], 'x')
	secondAt := strings.IndexByte(lines[1], 'y')
	if firstAt != secondAt {
		t.Errorf("subject starts at column %d after %q, %d after %q, want the same column",
			firstAt, "Writing", secondAt, "Resolving")
	}
}
