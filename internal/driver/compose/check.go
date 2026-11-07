package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Moq77111113/vessel/pkg/driver"
)

var ErrRootReadOnly = errors.New("cannot write")

func (m *Machine) Check(ctx context.Context) error {
	problems := m.docker.ready(ctx)
	if err := checkWritable(m.root); err != nil {
		problems = append(problems, err)
	}
	if len(problems) == 0 {
		return nil
	}
	format := "%w:" + strings.Repeat("\n  %w", len(problems))
	args := append([]any{driver.ErrNotReady}, anys(problems)...)
	return fmt.Errorf(format, args...)
}

func anys(problems []error) []any {
	list := make([]any, len(problems))
	for i, problem := range problems {
		list[i] = problem
	}
	return list
}

func checkWritable(root string) error {
	target := filepath.Join(root, composePath)
	dir := target
	for {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return fmt.Errorf("%w to %s", ErrRootReadOnly, target)
		}
		dir = parent
	}
	probe, err := os.CreateTemp(dir, ".vessel-probe-*")
	if err != nil {
		return fmt.Errorf("%w to %s", ErrRootReadOnly, target)
	}
	probe.Close()
	if err := os.Remove(probe.Name()); err != nil {
		return fmt.Errorf("%w to %s: %v", ErrRootReadOnly, target, err)
	}
	return nil
}
