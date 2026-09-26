package machine

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

// ErrKilled says a signal stopped a command, so what it did is unknown.
var ErrKilled = errors.New("command killed by a signal")

// Capture runs one shell command and returns what it printed on stdout alone.
type Capture func(ctx context.Context, command string) ([]byte, error)

// Sh runs a command through the system shell, returning stdout as the value and stderr with the error.
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
	if killed(err) {
		return nil, fmt.Errorf("%s: %w: %w", command, ErrKilled, err)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %w: %s", command, ErrAction, err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

// killed reports a signal stopping the shell itself, or a child it reports as 128 plus the signal, as POSIX shells do.
func killed(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	return exit.ExitCode() == -1 || exit.ExitCode() > 128
}

// DryMark opens the value Dry gives in place of a command's output.
const DryMark = "<not run: "

// Dry answers a command with a mark naming it, and never runs it.
func Dry(_ context.Context, command string) ([]byte, error) {
	return []byte(DryMark + command + ">"), nil
}
