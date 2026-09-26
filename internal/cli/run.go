package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// scratch is where a run unpacks when TMPDIR names nowhere: /tmp is often a tmpfs, so a bundle there sits in memory.
const scratch = "/var/tmp"

// Run executes the command line as this process, unpacking on disk and stopping cleanly on SIGINT or SIGTERM.
func Run() error {
	if os.Getenv("TMPDIR") == "" {
		os.Setenv("TMPDIR", scratch)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return New().ExecuteContext(ctx)
}
