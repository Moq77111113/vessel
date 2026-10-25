package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

const scratch = "/var/tmp"

func Run() error {
	useScratch(scratch)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return New().ExecuteContext(ctx)
}

func useScratch(dir string) {
	if os.Getenv("TMPDIR") != "" {
		return
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
	}
	os.Setenv("TMPDIR", dir)
}
