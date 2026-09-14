// Package site is what this machine answers for a delivery: the values it holds, and how a run resolves them.
package site

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/internal/atomicfile"
)

var ErrValueHasANewline = errors.New("a value cannot hold a newline")

// Store is the site values one delivery already holds on this machine.
type Store struct {
	path string
}

// NewStore returns the value store of a delivery under its directory.
func NewStore(dir string) *Store {
	return &Store{path: filepath.Join(dir, "values")}
}

// Path is where this store keeps the values.
func (v *Store) Path() string { return v.path }

// Read returns the values this machine holds, empty on a machine that holds none.
func (v *Store) Read() (map[string]string, error) {
	data, err := os.ReadFile(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", v.path, err)
	}
	values := map[string]string{}
	// A name never carries "=": descriptor.Read imposes ^[A-Z][A-Z0-9_]*$.
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok {
			values[name] = value
		}
	}
	return values, nil
}

// Write replaces the store with these values.
func (v *Store) Write(values map[string]string) error {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if strings.Contains(values[name], "\n") {
			return fmt.Errorf("%s: %w", name, ErrValueHasANewline)
		}
	}

	var body strings.Builder
	for _, name := range names {
		fmt.Fprintf(&body, "%s=%s\n", name, values[name])
	}
	dir := filepath.Dir(v.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return atomicfile.Write(v.path, []byte(body.String()), 0o600)
}
