package compose

import (
	"context"
	"strings"
	"testing"
)

func TestExecKeepsTheWarningsOutOfTheOutput(t *testing.T) {
	output, err := Exec(context.Background(), line("sh", "-c", "echo sensor; echo warning >&2"))
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if got, want := string(output), "sensor\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExecReturnsWhatAFailedCommandSaid(t *testing.T) {
	output, err := Exec(context.Background(), line("sh", "-c", "echo no such image >&2; exit 1"))
	if err == nil {
		t.Fatal("got no error, want the exit status")
	}
	if !strings.Contains(string(output), "no such image") {
		t.Errorf("got %q, want the error text", output)
	}
}
