package machine

import (
	"context"
	"io"
	"os/exec"
)

// Run executes one command, feeding it stdin, and returns everything it printed.
type Run func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)

// Exec runs a command for real, and is what a machine takes outside tests.
func Exec(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = stdin
	return command.CombinedOutput()
}
