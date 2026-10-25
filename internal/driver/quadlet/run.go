package quadlet

import (
	"context"
	"io"
	"os/exec"
)

type Command struct {
	Line  []string
	Stdin io.Reader
}

type Run func(ctx context.Context, command Command) ([]byte, error)

func Exec(ctx context.Context, command Command) ([]byte, error) {
	process := exec.CommandContext(ctx, command.Line[0], command.Line[1:]...)
	process.Stdin = command.Stdin
	return process.CombinedOutput()
}

func line(words ...string) Command { return Command{Line: words} }
