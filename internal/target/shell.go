package target

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrAction says a command the delivery declared did not succeed.
var ErrAction = errors.New("command failed")

// Capture runs one shell command and returns what it printed on stdout alone.
type Capture func(ctx context.Context, command string) ([]byte, error)

// Sh runs a command through the system shell, and is what NewShell takes outside tests.
//
// Only stdout comes back as a value. stderr rides with the error instead, so a command that
// chatters on stderr does not poison the value, and a failure still says why.
func Sh(ctx context.Context, command string) ([]byte, error) {
	shell := exec.CommandContext(ctx, "sh", "-c", command)
	var stderr bytes.Buffer
	shell.Stderr = &stderr
	out, err := shell.Output()
	if err != nil {
		return stderr.Bytes(), err
	}
	return out, nil
}

// Shell runs the commands a delivery declares.
type Shell struct {
	capture Capture
}

// NewShell returns a shell driving the given command runner.
func NewShell(capture Capture) *Shell { return &Shell{capture: capture} }

// Value runs a command and returns its output as the value of a variable.
func (s *Shell) Value(ctx context.Context, command string) (string, error) {
	output, err := s.run(ctx, command)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// Do runs a command for its effect.
func (s *Shell) Do(ctx context.Context, command string) error {
	_, err := s.run(ctx, command)
	return err
}

func (s *Shell) run(ctx context.Context, command string) ([]byte, error) {
	output, err := s.capture(ctx, command)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", command, ErrAction, strings.TrimSpace(string(output)))
	}
	return output, nil
}
