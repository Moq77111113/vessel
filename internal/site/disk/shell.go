package disk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"
)

var ErrAction = errors.New("command failed")

var ErrKilled = errors.New("command killed by a signal")

var ErrOutput = errors.New("command ran, its output could not be shown")

const tailLines = 10

// Both tails together fill one 80x24 terminal: the error an operator reads without scrolling.
const tailBytes = 80 * 24 / 2

type Run func(ctx context.Context, command string, stdout, stderr io.Writer) error

func Sh(ctx context.Context, command string, stdout, stderr io.Writer) error {
	shell := exec.CommandContext(ctx, "sh", "-c", command)
	shell.Stdout = stdout
	shell.Stderr = stderr
	return shell.Run()
}

type Shell struct {
	run Run
}

func NewShell(run Run) *Shell { return &Shell{run: run} }

func (s *Shell) Value(ctx context.Context, command string) (string, error) {
	var stdout bytes.Buffer
	var stderr tail
	if err := s.run(ctx, command, &stdout, &stderr); err != nil {
		return "", failure(command, err, stderr.section("stderr"))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (s *Shell) Stream(ctx context.Context, command string, out io.Writer) error {
	view := &screen{out: out}
	var stdout, stderr tail
	if err := s.run(ctx, command, io.MultiWriter(view, &stdout), io.MultiWriter(view, &stderr)); err != nil {
		return errors.Join(failure(command, err, stdout.section("stdout")+stderr.section("stderr")), view.end())
	}
	if err := view.end(); err != nil {
		return fmt.Errorf("%s: %w: %w", command, ErrOutput, err)
	}
	return nil
}

func (s *Shell) Mute(ctx context.Context, command string) error {
	if err := s.run(ctx, command, io.Discard, io.Discard); err != nil {
		return failure(command, err, "")
	}
	return nil
}

func failure(command string, err error, tails string) error {
	cause := ErrAction
	if killed(err) {
		cause = ErrKilled
	}
	return fmt.Errorf("%s: %w: %w%s", command, cause, err, tails)
}

func killed(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return false
	}
	return exit.ExitCode() == -1 || exit.ExitCode() > 128
}

type screen struct {
	lock sync.Mutex
	out  io.Writer
	err  error
	last byte
}

func (s *screen) Write(p []byte) (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.err != nil || len(p) == 0 {
		return len(p), nil
	}
	_, s.err = s.out.Write(p)
	s.last = p[len(p)-1]
	return len(p), nil
}

func (s *screen) end() error {
	if s.err == nil && s.last != 0 && s.last != '\n' {
		_, s.err = io.WriteString(s.out, "\n")
	}
	return s.err
}

type tail struct {
	data []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.data = append(t.data, p[max(0, len(p)-tailBytes):]...)
	lines := 0
	for index := len(t.data) - 2; index >= 0; index-- {
		if t.data[index] != '\n' {
			continue
		}
		lines++
		if lines == tailLines {
			t.data = t.data[index+1:]
			break
		}
	}
	if len(t.data) > tailBytes {
		t.data = t.data[len(t.data)-tailBytes:]
	}
	for len(t.data) > 0 && !utf8.RuneStart(t.data[0]) {
		t.data = t.data[1:]
	}
	return len(p), nil
}

func (t *tail) section(name string) string {
	text := strings.TrimRight(string(t.data), "\n")
	if text == "" {
		return ""
	}
	return "\n" + name + ":\n" + text
}

const DryMark = "<not run: "

func Dry(_ context.Context, command string, stdout, _ io.Writer) error {
	_, err := io.WriteString(stdout, DryMark+command+">")
	return err
}
