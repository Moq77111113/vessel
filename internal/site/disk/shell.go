package disk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrAction = errors.New("command failed")

var ErrKilled = errors.New("command killed by a signal")

type Capture func(ctx context.Context, command string) ([]byte, error)

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

type Shell struct {
	capture Capture
}

func NewShell(capture Capture) *Shell { return &Shell{capture: capture} }

func (s *Shell) Value(ctx context.Context, command string) (string, error) {
	output, err := s.run(ctx, command)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

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

func killed(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	return exit.ExitCode() == -1 || exit.ExitCode() > 128
}

const DryMark = "<not run: "

func Dry(_ context.Context, command string) ([]byte, error) {
	return []byte(DryMark + command + ">"), nil
}
