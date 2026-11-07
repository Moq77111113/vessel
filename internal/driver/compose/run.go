package compose

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

type Command struct {
	Line  []string
	Stdin io.Reader
}

type Run func(ctx context.Context, command Command) ([]byte, error)

// Exec keeps stderr apart: compose writes warnings there even when it succeeds.
func Exec(ctx context.Context, command Command) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	process := exec.CommandContext(ctx, command.Line[0], command.Line[1:]...)
	process.Stdin = command.Stdin
	process.Stdout = &stdout
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		return append(stderr.Bytes(), stdout.Bytes()...), err
	}
	return stdout.Bytes(), nil
}

func line(words ...string) Command { return Command{Line: words} }
