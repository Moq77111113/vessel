package values

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

func newValues(t *testing.T, set map[string]string, secrets []string) Values {
	t.Helper()
	shell := disk.NewShell(func(ctx context.Context, command string) ([]byte, error) {
		return []byte("from-command"), nil
	})
	return Values{Store: siteFor(t, t.TempDir()), Shell: shell, Set: set, Secrets: Secrets{Names: secrets, Remove: func(name string) string { return "remove-secret " + name }}}
}

func TestResolvePrefersWhatTheOperatorPassed(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Description: "address", From: "printf other"}}
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
	variables := []delivery.Variable{{Name: "DB_PASSWORD", From: "openssl rand -hex 32", Secret: true}}
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
	variables := []delivery.Variable{{Name: "DB_PASSWORD", Secret: true}}
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
	shell := disk.NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })
	r := Values{Store: values, Shell: shell, Set: nil, Secrets: Secrets{}}

	variables := []delivery.Variable{{Name: "DB_PASSWORD", Secret: true}}
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
	shell := disk.NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })
	r := Values{Store: values, Shell: shell, Set: map[string]string{"PUBLIC_HOST": "given"}, Secrets: Secrets{}}

	variables := []delivery.Variable{{Name: "PUBLIC_HOST"}}
	got, err := r.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Values["PUBLIC_HOST"] != "given" {
		t.Errorf("got %q, want %q", got.Values["PUBLIC_HOST"], "given")
	}
}

func TestResolveNamesTheVariableItCannotAnswerAndWhatThatVariableIs(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Description: "the public address of this machine"}}
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
	variables := []delivery.Variable{{Name: "DB_PASSWORD", From: "printf x", Secret: true}}
	got, err := newValues(t, nil, []string{"DB_PASSWORD"}).Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Secrets) != 0 {
		t.Errorf("got %v, want nothing to create", got.Secrets)
	}
}

func TestResolveReusesTheValueTheFirstInstallStored(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Description: "public address"}}
	values := siteFor(t, t.TempDir())
	shell := disk.NewShell(func(ctx context.Context, command string) ([]byte, error) { return nil, nil })

	first := Values{Store: values, Shell: shell, Set: map[string]string{"PUBLIC_HOST": "dmas.acme.local"}, Secrets: Secrets{}}
	got, err := first.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := values.Write(got.Values); err != nil {
		t.Fatalf("Write: %v", err)
	}

	second := Values{Store: values, Shell: shell, Set: nil, Secrets: Secrets{}}
	again, err := second.Resolve(context.Background(), variables)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if again.Values["PUBLIC_HOST"] != "dmas.acme.local" {
		t.Errorf("got %q, want the value the first install stored", again.Values["PUBLIC_HOST"])
	}
}

func TestResolveRefusesASetTheDeliveryNeverDeclared(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", Description: "address"}}
	_, err := newValues(t, map[string]string{"TYPO_HOST": "x"}, nil).
		Resolve(context.Background(), variables)
	if !errors.Is(err, ErrSetIsUnknown) {
		t.Fatalf("got %v, want ErrSetIsUnknown", err)
	}
	if !strings.Contains(err.Error(), "TYPO_HOST") {
		t.Errorf("got %q, want it to name TYPO_HOST", err.Error())
	}
}

func TestResolveRefusesToReplaceASecretTheMachineHolds(t *testing.T) {
	variables := []delivery.Variable{{Name: "DB_PASSWORD", From: "printf x", Secret: true}}
	_, err := newValues(t, map[string]string{"DB_PASSWORD": "new"}, []string{"DB_PASSWORD"}).
		Resolve(context.Background(), variables)
	if !errors.Is(err, ErrSecretHeld) {
		t.Fatalf("got %v, want ErrSecretHeld", err)
	}
	if !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Errorf("got %q, want it to name DB_PASSWORD", err.Error())
	}
	if !strings.Contains(err.Error(), "sudo remove-secret DB_PASSWORD") {
		t.Errorf("got %q, want it to name a command that reaches root's store", err.Error())
	}
}

func TestResolveNamesAFailedFromCommandAsNoValueNotAnAction(t *testing.T) {
	variables := []delivery.Variable{{Name: "PUBLIC_HOST", From: "false"}}
	shell := disk.NewShell(func(ctx context.Context, command string) ([]byte, error) {
		return []byte("boom"), errors.New("exit status 1")
	})
	values := Values{Store: siteFor(t, t.TempDir()), Shell: shell, Set: nil, Secrets: Secrets{}}
	_, err := values.Resolve(context.Background(), variables)
	if !errors.Is(err, ErrNoValue) {
		t.Fatalf("got %v, want ErrNoValue: a failed from: runs before anything is written", err)
	}
	if errors.Is(err, disk.ErrAction) {
		t.Errorf("got disk.ErrAction, want only ErrNoValue: a from: failure is not a partway action")
	}
}

func TestResolveRefusesAPlainValueHoldingAControlCharacter(t *testing.T) {
	cases := map[string]struct {
		set    map[string]string
		output string
	}{
		"set carriage return": {set: map[string]string{"PUBLIC_HOST": "a\rb"}},
		"set nul":             {set: map[string]string{"PUBLIC_HOST": "a\x00b"}},
		"from newline":        {output: "a\nb"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			shell := disk.NewShell(func(context.Context, string) ([]byte, error) { return []byte(c.output), nil })
			values := Values{Store: siteFor(t, t.TempDir()), Shell: shell, Set: c.set, Secrets: Secrets{}}
			variables := []delivery.Variable{{Name: "PUBLIC_HOST", From: "hostname -f"}}
			if _, err := values.Resolve(context.Background(), variables); !errors.Is(err, ErrValueHasAControl) {
				t.Fatalf("got %v, want ErrValueHasAControl", err)
			}
		})
	}
}
