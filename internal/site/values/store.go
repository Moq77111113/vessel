package values

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/Moq77111113/vessel/internal/atomicfile"
)

var ErrValueHasAControl = errors.New("a value cannot hold a control character")

type Store struct {
	path string
}

func NewStore(dir string) *Store {
	return &Store{path: filepath.Join(dir, "values")}
}

func (v *Store) Path() string { return v.path }

func (v *Store) Read() (map[string]string, error) {
	data, err := os.ReadFile(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", v.path, err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok {
			values[name] = value
		}
	}
	return values, nil
}

func (v *Store) Write(values map[string]string) error {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := checkLine(name, values[name]); err != nil {
			return err
		}
	}

	var body strings.Builder
	for _, name := range names {
		fmt.Fprintf(&body, "%s=%s\n", name, values[name])
	}
	dir := filepath.Dir(v.path)
	if err := atomicfile.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return atomicfile.Write(v.path, []byte(body.String()), 0o600)
}

func checkLine(name, value string) error {
	if strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\t' }) {
		return fmt.Errorf("%s: %w", name, ErrValueHasAControl)
	}
	return nil
}
