package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tihaya-anon/ai-infra-lab/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	os.Exit(worker.Run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
