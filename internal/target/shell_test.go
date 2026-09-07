package target

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValueTakesTheCommandOutputWithoutItsNewline(t *testing.T) {
	shell := NewShell(func(ctx context.Context, command string) ([]byte, error) {
		return []byte("a3f9\n"), nil
	})
	got, err := shell.Value(context.Background(), "openssl rand -hex 2")
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got != "a3f9" {
		t.Errorf("got %q, want %q", got, "a3f9")
	}
}

func TestDoNamesTheCommandThatFailed(t *testing.T) {
	shell := NewShell(func(ctx context.Context, command string) ([]byte, error) {
		return []byte("permission denied"), errors.New("exit status 1")
	})
	err := shell.Do(context.Background(), "mkdir /etc/acme")
	if !errors.Is(err, ErrAction) {
		t.Fatalf("got %v, want ErrAction", err)
	}
	if got := err.Error(); !strings.Contains(got, "mkdir /etc/acme") {
		t.Errorf("got %q, want it to name the command", got)
	}
}

func TestShRunsARealCommand(t *testing.T) {
	got, err := NewShell(Sh).Value(context.Background(), "printf hello")
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestShKeepsStderrOutOfTheValue(t *testing.T) {
	got, err := NewShell(Sh).Value(context.Background(), "printf noise >&2; printf hello")
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestShSaysWhyARealCommandFailed(t *testing.T) {
	_, err := NewShell(Sh).Value(context.Background(), "printf 'no such thing' >&2; exit 1")
	if !errors.Is(err, ErrAction) {
		t.Fatalf("got %v, want ErrAction", err)
	}
	if !strings.Contains(err.Error(), "no such thing") {
		t.Errorf("got %q, want it to carry what the command printed", err.Error())
	}
}
