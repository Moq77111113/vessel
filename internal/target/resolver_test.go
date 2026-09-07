package target

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/delivery"
)

func resolver(t *testing.T, set map[string]string, secrets []string, answers string) *Resolver {
	t.Helper()
	shell := NewShell(func(ctx context.Context, command string) ([]byte, error) {
		return []byte("from-command"), nil
	})
	return NewResolver(NewValues(t.TempDir(), "acme"), shell, set, secrets,
		io.Discard, strings.NewReader(answers))
}

func TestResolvePrefersWhatTheOperatorPassed(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Ask: "address", From: "printf other"}}
	got, err := resolver(t, map[string]string{"PUBLIC_HOST": "given"}, nil, "").
		Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Values["PUBLIC_HOST"] != "given" {
		t.Errorf("got %q, want %q", got.Values["PUBLIC_HOST"], "given")
	}
}

func TestResolveRunsTheCommandWhenNothingElseAnswers(t *testing.T) {
	variables := []delivery.Variable{{Name: "DB_PASSWORD", From: "openssl rand -hex 32", Secret: true}}
	got, err := resolver(t, nil, nil, "").Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Secrets["DB_PASSWORD"] != "from-command" {
		t.Errorf("got %q, want %q", got.Secrets["DB_PASSWORD"], "from-command")
	}
	if _, ok := got.Values["DB_PASSWORD"]; ok {
		t.Error("a secret must not land in the plain values")
	}
}

func TestResolveAsksTheOperatorWhenThereIsNoCommand(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Ask: "public address"}}
	got, err := resolver(t, nil, nil, "dmas.acme.local\n").Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Values["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %q, want %q", got.Values["PUBLIC_HOST"], "dmas.acme.local")
	}
}

func TestResolveAcceptsAnAnswerWithNoTrailingNewline(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Ask: "public address"}}
	got, err := resolver(t, nil, nil, "dmas.acme.local").Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Values["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %q, want %q", got.Values["PUBLIC_HOST"], "dmas.acme.local")
	}
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestResolveReportsAReadFailureRatherThanNoValue(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Ask: "public address"}}
	readErr := errors.New("terminal went away")
	shell := NewShell(func(ctx context.Context, command string) ([]byte, error) {
		return []byte("from-command"), nil
	})
	r := NewResolver(NewValues(t.TempDir(), "acme"), shell, nil, nil, io.Discard, failingReader{err: readErr})

	_, err := r.Resolve(context.Background(), variables)
	if !errors.Is(err, readErr) {
		t.Fatalf("got %v, want it to wrap %v", err, readErr)
	}
	if errors.Is(err, ErrNoValue) {
		t.Errorf("got ErrNoValue, want the read failure reported instead")
	}
}

func TestResolveKeepsASecretPassedWithSetOutOfThePlainValues(t *testing.T) {
	variables := []delivery.Variable{{Name: "DB_PASSWORD", Secret: true}}
	got, err := resolver(t, map[string]string{"DB_PASSWORD": "x"}, nil, "").
		Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Secrets["DB_PASSWORD"] != "x" {
		t.Errorf("got %q, want %q", got.Secrets["DB_PASSWORD"], "x")
	}
	if _, ok := got.Values["DB_PASSWORD"]; ok {
		t.Error("a secret passed with --set must not land in the plain values")
	}
}

func TestResolveNeverAnswersASecretFromTheStore(t *testing.T) {
	values := NewValues(t.TempDir(), "acme")
	if err := values.Write(map[string]string{"DB_PASSWORD": "stored"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	shell := NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })
	r := NewResolver(values, shell, nil, nil, io.Discard, strings.NewReader(""))

	variables := []delivery.Variable{{Name: "DB_PASSWORD", Secret: true}}
	_, err := r.Resolve(context.Background(), variables)
	if !errors.Is(err, ErrNoValue) {
		t.Fatalf("got %v, want ErrNoValue: a stored value must never answer a secret", err)
	}
}

func TestResolveSetOutranksAStoredValue(t *testing.T) {
	values := NewValues(t.TempDir(), "acme")
	if err := values.Write(map[string]string{"PUBLIC_HOST": "stored"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	shell := NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })
	r := NewResolver(values, shell, map[string]string{"PUBLIC_HOST": "given"}, nil, io.Discard, strings.NewReader(""))

	variables := []delivery.Variable{{Name: "PUBLIC_HOST"}}
	got, err := r.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Values["PUBLIC_HOST"] != "given" {
		t.Errorf("got %q, want %q", got.Values["PUBLIC_HOST"], "given")
	}
}

func TestResolveNamesAVariableItCannotAnswer(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST"}}
	_, err := resolver(t, nil, nil, "").Resolve(context.Background(), variables)
	if !errors.Is(err, ErrNoValue) {
		t.Fatalf("got %v, want ErrNoValue", err)
	}
	if !strings.Contains(err.Error(), "PUBLIC_HOST") {
		t.Errorf("got %q, want it to name PUBLIC_HOST", err.Error())
	}
}

func TestResolveLeavesASecretTheMachineAlreadyHolds(t *testing.T) {
	variables := []delivery.Variable{{Name: "DB_PASSWORD", From: "printf x", Secret: true}}
	got, err := resolver(t, nil, []string{"DB_PASSWORD"}, "").Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Secrets) != 0 {
		t.Errorf("got %v, want nothing to create", got.Secrets)
	}
}

func TestResolveDoesNotAskTwiceAcrossTwoInstalls(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Ask: "public address"}}
	values := NewValues(t.TempDir(), "acme")
	shell := NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })

	first := NewResolver(values, shell, nil, nil, io.Discard, strings.NewReader("dmas.acme.local\n"))
	got, err := first.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := values.Write(got.Values); err != nil {
		t.Fatalf("Write: %v", err)
	}

	second := NewResolver(values, shell, nil, nil, io.Discard, strings.NewReader(""))
	again, err := second.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if again.Values["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %q, want the value the first install stored", again.Values["PUBLIC_HOST"])
	}
}

func TestResolveRefusesASetTheDeliveryNeverDeclared(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Ask: "address"}}
	_, err := resolver(t, map[string]string{"TYPO_HOST": "x"}, nil, "dmas\n").
		Resolve(context.Background(), variables)
	if !errors.Is(err, ErrSetIsUnknown) {
		t.Fatalf("got %v, want ErrSetIsUnknown", err)
	}
	if !strings.Contains(err.Error(), "TYPO_HOST") {
		t.Errorf("got %q, want it to name TYPO_HOST", err.Error())
	}
}

// install runs as root, so the secret sits in root's store: the command the error names must
// carry sudo, or an operator running it unprivileged reaches an empty store and finds nothing.
func TestResolveRefusesToReplaceASecretTheMachineHolds(t *testing.T) {
	variables := []delivery.Variable{{Name: "DB_PASSWORD", From: "printf x", Secret: true}}
	_, err := resolver(t, map[string]string{"DB_PASSWORD": "new"}, []string{"DB_PASSWORD"}, "").
		Resolve(context.Background(), variables)
	if !errors.Is(err, ErrSecretHeld) {
		t.Fatalf("got %v, want ErrSecretHeld", err)
	}
	if !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Errorf("got %q, want it to name DB_PASSWORD", err.Error())
	}
	if !strings.Contains(err.Error(), "sudo podman secret rm") {
		t.Errorf("got %q, want it to name a command that reaches root's store", err.Error())
	}
}
