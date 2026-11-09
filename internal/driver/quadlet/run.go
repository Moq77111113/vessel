package quadlet

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
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

func syncFileSystem(ctx context.Context, run Run, dir string) error {
	output, err := run(ctx, line("sync", "--file-system", dir))
	if err != nil {
		return fmt.Errorf("sync --file-system %s: %w: %s", dir, ErrCommand, strings.TrimSpace(string(output)))
	}
	return nil
}
