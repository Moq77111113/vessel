package values

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

var (
	ErrNoValue      = errors.New("is not set")
	ErrSetIsUnknown = errors.New("--set names a variable this delivery never declared")
	ErrSecretHeld   = errors.New("this machine already holds this secret, --set cannot replace it")
)

type Resolution struct {
	Values  map[string]string
	Secrets map[string]string
}

type Shell interface {
	Value(ctx context.Context, command string) (string, error)
}

type Secrets struct {
	Names  []string
	Remove func(secret string) string
}

type Values struct {
	site    *Store
	shell   Shell
	set     map[string]string
	secrets map[string]bool
	remove  func(secret string) string
}

func NewValues(site *Store, shell Shell, set map[string]string, secrets Secrets) *Values {
	machineSecrets := make(map[string]bool, len(secrets.Names))
	for _, name := range secrets.Names {
		machineSecrets[name] = true
	}
	return &Values{site: site, shell: shell, set: set, secrets: machineSecrets, remove: secrets.Remove}
}

func (v *Values) Resolve(ctx context.Context, variables []delivery.Variable) (Resolution, error) {
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

func (v *Values) checkSet(variables []delivery.Variable) error {
	byName := make(map[string]delivery.Variable, len(variables))
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
			return fmt.Errorf("--set %s: remove it with sudo %s, then install again: %w",
				name, v.remove(name), ErrSecretHeld)
		}
	}
	return nil
}

func (v *Values) value(ctx context.Context, variable delivery.Variable,
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

func noValue(variable delivery.Variable) error {
	if variable.Description == "" {
		return fmt.Errorf("%s %w", variable.Name, ErrNoValue)
	}
	return fmt.Errorf("%s %w: %s", variable.Name, ErrNoValue, variable.Description)
}
