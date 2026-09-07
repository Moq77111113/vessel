package target

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// valuesPath is where a machine keeps the site values it was given, under the target root.
const valuesPath = "var/lib/vessel"

// ErrValueHasANewline says a value cannot be stored because the file keeps one value per line.
var ErrValueHasANewline = errors.New("a value cannot hold a newline")

// Values is the site values one delivery already holds on this machine.
type Values struct {
	path string
}

// NewValues returns the value store of a delivery under the target root.
func NewValues(root, name string) *Values {
	return &Values{path: filepath.Join(root, valuesPath, name, "values")}
}

// Read returns the values this machine holds, empty on a machine that holds none.
func (v *Values) Read() (map[string]string, error) {
	data, err := os.ReadFile(v.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", v.path, err)
	}
	values := map[string]string{}
	// Cut on the first "=": relies on delivery.Read imposing ^[A-Z][A-Z0-9_]*$ on names, so a name
	// never carries one.
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok {
			values[name] = value
		}
	}
	return values, nil
}

// Write replaces the store with these values.
func (v *Values) Write(values map[string]string) error {
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
	if err := os.WriteFile(v.path, []byte(body.String()), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", v.path, err)
	}
	return nil
}
