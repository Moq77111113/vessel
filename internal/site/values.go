package site

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// Errors Resolve returns.
var (
	ErrNoValue      = errors.New("is not set")
	ErrSetIsUnknown = errors.New("--set names a variable this delivery never declared")
	ErrSecretHeld   = errors.New("this machine already holds this secret, --set cannot replace it")
)

// Resolution is what one install run knows: the values files carry, and the secrets to create.
type Resolution struct {
	Values  map[string]string
	Secrets map[string]string
}

// Shell answers the command a variable's from: names.
type Shell interface {
	Value(ctx context.Context, command string) (string, error)
}

// Values answers the variables a delivery declares, from this machine and this operator.
type Values struct {
	site    *Store
	shell   Shell
	set     map[string]string
	secrets map[string]bool
}

// NewValues returns the values of one run: --set, what a previous install stored, then from:.
func NewValues(site *Store, shell Shell, set map[string]string, secrets []string) *Values {
	machineSecrets := make(map[string]bool, len(secrets))
	for _, name := range secrets {
		machineSecrets[name] = true
	}
	return &Values{site: site, shell: shell, set: set, secrets: machineSecrets}
}

// Resolve answers every variable, or names the first one it cannot.
func (v *Values) Resolve(ctx context.Context, variables []descriptor.Variable) (Resolution, error) {
	values, err := v.site.Read()
	if err != nil {
		return Resolution{}, err
	}
	if err := v.checkSet(variables); err != nil {
		return Resolution{}, err
	}
	resolution := Resolution{Values: map[string]string{}, Secrets: map[string]string{}}
	for _, variable := range variables {
		if variable.Secret && v.secrets[variable.Name] {
			continue
		}
		value, err := v.value(ctx, variable, values)
		if err != nil {
			return Resolution{}, err
		}
		if variable.Secret {
			resolution.Secrets[variable.Name] = value
			continue
		}
		if err := checkLine(variable.Name, value); err != nil {
			return Resolution{}, err
		}
		resolution.Values[variable.Name] = value
	}
	return resolution, nil
}

// checkSet refuses a --set naming a variable this delivery never declared, or a secret this machine holds.
func (v *Values) checkSet(variables []descriptor.Variable) error {
	byName := make(map[string]descriptor.Variable, len(variables))
	for _, variable := range variables {
		byName[variable.Name] = variable
	}
	names := make([]string, 0, len(v.set))
	for name := range v.set {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		variable, ok := byName[name]
		if !ok {
			return fmt.Errorf("--set %s: %w", name, ErrSetIsUnknown)
		}
		if variable.Secret && v.secrets[name] {
			return fmt.Errorf("--set %s: remove it with sudo podman secret rm %s, then install again: %w",
				name, name, ErrSecretHeld)
		}
	}
	return nil
}

func (v *Values) value(ctx context.Context, variable descriptor.Variable,
	values map[string]string) (string, error) {
	if value, ok := v.set[variable.Name]; ok {
		return value, nil
	}
	if value, ok := values[variable.Name]; ok && !variable.Secret {
		return value, nil
	}
	if variable.From != "" {
		value, err := v.shell.Value(ctx, variable.From)
		if err != nil {
			return "", fmt.Errorf("%s %w: %s", variable.Name, ErrNoValue, err)
		}
		return value, nil
	}
	return "", noValue(variable)
}

// noValue refuses a variable nothing answers, naming it and what it is.
func noValue(variable descriptor.Variable) error {
	if variable.Description == "" {
		return fmt.Errorf("%s %w", variable.Name, ErrNoValue)
	}
	return fmt.Errorf("%s %w: %s", variable.Name, ErrNoValue, variable.Description)
}
