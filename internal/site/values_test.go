package site

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
)

func newValues(t *testing.T, set map[string]string, secrets []string) *Values {
	t.Helper()
	shell := machine.NewShell(func(ctx context.Context, command string) ([]byte, error) {
		return []byte("from-command"), nil
	})
	return NewValues(siteFor(t, t.TempDir()), shell, set, secrets)
}

func TestResolvePrefersWhatTheOperatorPassed(t *testing.T) {
	variables := []descriptor.Variable{{Name: "PUBLIC_HOST", Description: "address", From: "printf other"}}
	got, err := newValues(t, map[string]string{"PUBLIC_HOST": "given"}, nil).
		Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Values["PUBLIC_HOST"] != "given" {
		t.Errorf("got %q, want %q", got.Values["PUBLIC_HOST"], "given")
	}
}

func TestResolveRunsTheCommandWhenNothingElseAnswers(t *testing.T) {
	variables := []descriptor.Variable{{Name: "DB_PASSWORD", From: "openssl rand -hex 32", Secret: true}}
	got, err := newValues(t, nil, nil).Resolve(context.Background(), variables)
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

func TestResolveKeepsASecretPassedWithSetOutOfThePlainValues(t *testing.T) {
	variables := []descriptor.Variable{{Name: "DB_PASSWORD", Secret: true}}
	got, err := newValues(t, map[string]string{"DB_PASSWORD": "x"}, nil).
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
	values := siteFor(t, t.TempDir())
	if err := values.Write(map[string]string{"DB_PASSWORD": "stored"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	shell := machine.NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })
	r := NewValues(values, shell, nil, nil)

	variables := []descriptor.Variable{{Name: "DB_PASSWORD", Secret: true}}
	_, err := r.Resolve(context.Background(), variables)
	if !errors.Is(err, ErrNoValue) {
		t.Fatalf("got %v, want ErrNoValue: a stored value must never answer a secret", err)
	}
}

func TestResolveSetOutranksAStoredValue(t *testing.T) {
	values := siteFor(t, t.TempDir())
	if err := values.Write(map[string]string{"PUBLIC_HOST": "stored"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	shell := machine.NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })
	r := NewValues(values, shell, map[string]string{"PUBLIC_HOST": "given"}, nil)

	variables := []descriptor.Variable{{Name: "PUBLIC_HOST"}}
	got, err := r.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Values["PUBLIC_HOST"] != "given" {
		t.Errorf("got %q, want %q", got.Values["PUBLIC_HOST"], "given")
	}
}

func TestResolveNamesTheVariableItCannotAnswerAndWhatThatVariableIs(t *testing.T) {
	variables := []descriptor.Variable{{Name: "PUBLIC_HOST", Description: "the public address of this machine"}}
	_, err := newValues(t, nil, nil).Resolve(context.Background(), variables)
	if !errors.Is(err, ErrNoValue) {
		t.Fatalf("got %v, want ErrNoValue", err)
	}
	if !strings.Contains(err.Error(), "PUBLIC_HOST") {
		t.Errorf("got %q, want it to name PUBLIC_HOST", err.Error())
	}
	if !strings.Contains(err.Error(), "the public address of this machine") {
		t.Errorf("got %q, want it to carry the description", err.Error())
	}
}

func TestResolveLeavesASecretTheMachineAlreadyHolds(t *testing.T) {
	variables := []descriptor.Variable{{Name: "DB_PASSWORD", From: "printf x", Secret: true}}
	got, err := newValues(t, nil, []string{"DB_PASSWORD"}).Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Secrets) != 0 {
		t.Errorf("got %v, want nothing to create", got.Secrets)
	}
}

func TestResolveReusesTheValueTheFirstInstallStored(t *testing.T) {
	variables := []descriptor.Variable{{Name: "PUBLIC_HOST", Description: "public address"}}
	values := siteFor(t, t.TempDir())
	shell := machine.NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })

	first := NewValues(values, shell, map[string]string{"PUBLIC_HOST": "dmas.acme.local"}, nil)
	got, err := first.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := values.Write(got.Values); err != nil {
		t.Fatalf("Write: %v", err)
	}

	second := NewValues(values, shell, nil, nil)
	again, err := second.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if again.Values["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %q, want the value the first install stored", again.Values["PUBLIC_HOST"])
	}
}

func TestResolveRefusesASetTheDeliveryNeverDeclared(t *testing.T) {
	variables := []descriptor.Variable{{Name: "PUBLIC_HOST", Description: "address"}}
	_, err := newValues(t, map[string]string{"TYPO_HOST": "x"}, nil).
		Resolve(context.Background(), variables)
	if !errors.Is(err, ErrSetIsUnknown) {
		t.Fatalf("got %v, want ErrSetIsUnknown", err)
	}
	if !strings.Contains(err.Error(), "TYPO_HOST") {
		t.Errorf("got %q, want it to name TYPO_HOST", err.Error())
	}
}

// An install runs as root, so the command the error names has to carry sudo to reach the store.
func TestResolveRefusesToReplaceASecretTheMachineHolds(t *testing.T) {
	variables := []descriptor.Variable{{Name: "DB_PASSWORD", From: "printf x", Secret: true}}
	_, err := newValues(t, map[string]string{"DB_PASSWORD": "new"}, []string{"DB_PASSWORD"}).
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

func TestResolveNamesAFailedFromCommandAsNoValueNotAnAction(t *testing.T) {
	variables := []descriptor.Variable{{Name: "PUBLIC_HOST", From: "false"}}
	shell := machine.NewShell(func(ctx context.Context, command string) ([]byte, error) {
		return []byte("boom"), errors.New("exit status 1")
	})
	values := NewValues(siteFor(t, t.TempDir()), shell, nil, nil)
	_, err := values.Resolve(context.Background(), variables)
	if !errors.Is(err, ErrNoValue) {
		t.Fatalf("got %v, want ErrNoValue: a failed from: runs before anything is written", err)
	}
	if errors.Is(err, machine.ErrAction) {
		t.Errorf("got machine.ErrAction, want only ErrNoValue: a from: failure is not a partway action")
	}
}
