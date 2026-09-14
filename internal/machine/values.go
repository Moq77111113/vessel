package machine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/internal/delivery"
)

// Errors Resolve returns.
var (
	ErrNoValue      = errors.New("no value for this variable")
	ErrSetIsUnknown = errors.New("--set names a variable this delivery never declared")
	ErrSecretHeld   = errors.New("this machine already holds this secret, --set cannot replace it")
)

// Resolution is what one install run knows: the values files carry, and the secrets to create.
type Resolution struct {
	Values  map[string]string
	Secrets map[string]string
}

// Values answers the variables a delivery declares, from this machine and this operator.
type Values struct {
	site    *Site
	shell   *Shell
	set     map[string]string
	secrets map[string]bool
	out     io.Writer
	ask     *bufio.Reader
}

// NewValues returns the values of one run, read from this machine, this delivery and this operator.
func NewValues(site *Site, shell *Shell, set map[string]string, secrets []string,
	out io.Writer, ask io.Reader) *Values {
	machineSecrets := make(map[string]bool, len(secrets))
	for _, name := range secrets {
		machineSecrets[name] = true
	}
	return &Values{
		site:    site,
		shell:   shell,
		set:     set,
		secrets: machineSecrets,
		out:     out,
		ask:     bufio.NewReader(ask),
	}
}

// Resolve answers every variable, or names the first one it cannot.
func (v *Values) Resolve(ctx context.Context, variables []delivery.Variable) (Resolution, error) {
	values, err := v.site.Read()
	if err != nil {
		return Resolution{}, err
	}
	if err := v.checkTheSetLands(variables); err != nil {
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
		resolution.Values[variable.Name] = value
	}
	return resolution, nil
}

// checkTheSetLands refuses a --set that cannot reach the install: one naming a variable the
// delivery never declared, and one naming a secret this machine already holds.
func (v *Values) checkTheSetLands(variables []delivery.Variable) error {
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
			return fmt.Errorf("--set %s: remove it with sudo podman secret rm %s, then install again: %w",
				name, name, ErrSecretHeld)
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
		return v.shell.Value(ctx, variable.From)
	}
	if variable.Ask != "" {
		return v.question(variable)
	}
	return "", fmt.Errorf("%s: %w", variable.Name, ErrNoValue)
}

func (v *Values) question(variable delivery.Variable) (string, error) {
	fmt.Fprintf(v.out, "%s (%s): ", variable.Name, variable.Ask)
	line, err := v.ask.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read the answer for %s: %w", variable.Name, err)
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return "", fmt.Errorf("%s: %w", variable.Name, ErrNoValue)
	}
	return answer, nil
}
