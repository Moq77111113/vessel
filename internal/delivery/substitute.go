package delivery

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// Errors Substitute returns.
var (
	ErrUnknownVariable = errors.New("no variable declares this marker, declare it or remove the marker")
	ErrSecretInAFile   = errors.New("a secret cannot be written into a file, mount it with Secret=")
)

// marker is how a variable appears inside a carried file. Three hashes on each side collide with
// neither systemd specifiers nor INI, JSON or TOML syntax.
var marker = regexp.MustCompile(`###([A-Z][A-Z0-9_]*)###`)

// Substitute puts every value in place of its marker, and returns the files with new bytes.
// A marker naming a secret is refused: a secret reaches a container through Secret=, never as
// plain bytes on the disk.
func Substitute(files []descriptor.File, values map[string]string, secrets []string) ([]descriptor.File, error) {
	out := make([]descriptor.File, len(files))
	for i, file := range files {
		var missing, secret string
		data := marker.ReplaceAllFunc(file.Data, func(match []byte) []byte {
			name := string(marker.FindSubmatch(match)[1])
			if slices.Contains(secrets, name) {
				secret = name
				return match
			}
			value, ok := values[name]
			if !ok {
				missing = name
				return match
			}
			return []byte(value)
		})
		if secret != "" {
			return nil, fmt.Errorf("%s in %s: %w", secret, file.Path, ErrSecretInAFile)
		}
		if missing != "" {
			return nil, fmt.Errorf("%s in %s: %w", missing, file.Path, ErrUnknownVariable)
		}
		out[i] = descriptor.File{Path: file.Path, Data: data}
	}
	return out, nil
}

// CheckMarkers refuses a marker naming a variable the delivery never declared, or naming a secret.
// link runs it on the files it carries, so a typo is caught where the YAML can still be edited.
func CheckMarkers(files []descriptor.File, variables []Variable) error {
	names := make(map[string]bool, len(variables))
	for _, variable := range variables {
		names[variable.Name] = true
	}
	secrets := SecretNames(variables)
	for _, file := range files {
		for _, match := range marker.FindAllSubmatch(file.Data, -1) {
			name := string(match[1])
			if slices.Contains(secrets, name) {
				return fmt.Errorf("%s in %s: %w", name, file.Path, ErrSecretInAFile)
			}
			if !names[name] {
				return fmt.Errorf("%s in %s: %w", name, file.Path, ErrUnknownVariable)
			}
		}
	}
	return nil
}
