package quadlet

import (
	"context"
	"io"
	"os/exec"
)

type Run func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)

func Exec(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = stdin
	return command.CombinedOutput()
}
