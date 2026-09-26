package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// scratch is where a run unpacks when TMPDIR names nowhere: /tmp is often a tmpfs, so a bundle there sits in memory.
// A minimal container has no /var/tmp, and keeps its own temporary directory.
const scratch = "/var/tmp"

// Run executes the command line as this process, unpacking on disk and stopping cleanly on SIGINT or SIGTERM.
func Run() error {
	useScratch(scratch)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return New().ExecuteContext(ctx)
}

// useScratch points TMPDIR at dir when nothing names one and dir is there.
func useScratch(dir string) {
	if os.Getenv("TMPDIR") != "" {
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
	}
	os.Setenv("TMPDIR", dir)
}
