// Package delivery reads the declaration of a delivery: its files, its site values, its actions.
package delivery

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Name is the file a source directory declares its delivery in.
const Name = "vessel.yaml"

// Errors Read returns.
var (
	ErrNoDelivery        = errors.New("no delivery declaration")
	ErrNoName            = errors.New("the delivery carries no name")
	ErrDeliveryName      = errors.New("the delivery name is not a plain lowercase identifier")
	ErrVariableName      = errors.New("a variable name is not upper snake case")
	ErrTargetNotAbsolute = errors.New("a file target is not an absolute path")
	ErrTargetEscapes     = errors.New("a file target leaves the target root")
	ErrTargetNotClean    = errors.New("a file target is not a clean path")
	ErrSecretAsks        = errors.New("a secret is never asked on screen, declare a from: or pass --set")
	ErrTargetIsRoot      = errors.New("a file target names no file")
	ErrFileSource        = errors.New("a file carries no source")
)

// variableName is the shape a name takes, so a marker cannot be mistaken for prose.
var variableName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// deliveryName is the shape a delivery name takes: a single path segment, so it can name a
// directory under the target root without ever leaving it.
var deliveryName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// File is one plain file the delivery carries, and where it lands on the machine.
type File struct {
	Source string `yaml:"source" json:"source"`
	Target string `yaml:"target" json:"target"`
}

// Variable is a site value the machine supplies at install time.
type Variable struct {
	Name   string `yaml:"name" json:"name"`
	Ask    string `yaml:"ask,omitempty" json:"ask,omitempty"`
	From   string `yaml:"from,omitempty" json:"from,omitempty"`
	Secret bool   `yaml:"secret,omitempty" json:"secret,omitempty"`
}

// Delivery is what vessel.yaml declares.
type Delivery struct {
	Name      string     `yaml:"name"`
	Version   string     `yaml:"version"`
	Units     string     `yaml:"units"`
	Files     []File     `yaml:"files"`
	Variables []Variable `yaml:"variables"`
	Actions   []string   `yaml:"actions"`
}

// CheckName refuses a name that cannot stand for a directory under the target root.
func CheckName(name string) error {
	if name == "" {
		return fmt.Errorf("%s: %w", Name, ErrNoName)
	}
	if !deliveryName.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrDeliveryName)
	}
	return nil
}

// SecretNames names every secret a delivery declares.
func SecretNames(variables []Variable) []string {
	var names []string
	for _, variable := range variables {
		if variable.Secret {
			names = append(names, variable.Name)
		}
	}
	return names
}

// leaves reports whether a target walks back up out of the directory it names.
func leaves(target string) bool {
	for _, segment := range strings.Split(target, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// Read returns the declaration the directory carries, and ErrNoDelivery when it carries none.
func Read(dir fs.FS) (Delivery, error) {
	data, err := fs.ReadFile(dir, Name)
	if errors.Is(err, fs.ErrNotExist) {
		return Delivery{}, fmt.Errorf("%s: %w", Name, ErrNoDelivery)
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("read %s: %w", Name, err)
	}
	var declaration Delivery
	if err := yaml.Unmarshal(data, &declaration); err != nil {
		return Delivery{}, fmt.Errorf("read %s: %w", Name, err)
	}
	if err := CheckName(declaration.Name); err != nil {
		return Delivery{}, err
	}
	if declaration.Units == "" {
		declaration.Units = "."
	}
	for _, variable := range declaration.Variables {
		if !variableName.MatchString(variable.Name) {
			return Delivery{}, fmt.Errorf("%q: %w", variable.Name, ErrVariableName)
		}
		if variable.Secret && variable.Ask != "" {
			return Delivery{}, fmt.Errorf("%s: %w", variable.Name, ErrSecretAsks)
		}
	}
	for _, file := range declaration.Files {
		if file.Source == "" {
			return Delivery{}, fmt.Errorf("target %q: %w", file.Target, ErrFileSource)
		}
		if len(file.Target) == 0 || file.Target[0] != '/' {
			return Delivery{}, fmt.Errorf("%q: %w", file.Target, ErrTargetNotAbsolute)
		}
		if leaves(file.Target) {
			return Delivery{}, fmt.Errorf("%q: %w", file.Target, ErrTargetEscapes)
		}
		if path.Clean(file.Target) != file.Target {
			return Delivery{}, fmt.Errorf("%q: %w", file.Target, ErrTargetNotClean)
		}
		if file.Target == "/" {
			return Delivery{}, fmt.Errorf("%q: %w", file.Target, ErrTargetIsRoot)
		}
	}
	return declaration, nil
}
