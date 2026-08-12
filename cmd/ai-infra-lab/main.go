package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/tihaya-anon/ai-infra-lab/internal/controller"
	"github.com/tihaya-anon/ai-infra-lab/internal/kube"
	"github.com/tihaya-anon/ai-infra-lab/internal/scheduler"
	"k8s.io/client-go/rest"
)

func main() {
	component := flag.String("component", "all", "component to run: controller, scheduler, or all")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	config, err := kube.Config()
	if err != nil {
		logger.Error("build Kubernetes config", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, *component, config, logger); err != nil {
		logger.Error("component stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, component string, config *rest.Config, logger *slog.Logger) error {
	switch component {
	case "controller":
		return controller.New(config, logger).Run(ctx, 2)
	case "scheduler":
		return scheduler.New(config, logger).Run(ctx)
	case "worker":
		logger.Info("training worker started")
		<-ctx.Done()
		return nil
	case "all":
		errCh := make(chan error, 2)
		go func() { errCh <- controller.New(config, logger).Run(ctx, 2) }()
		go func() { errCh <- scheduler.New(config, logger).Run(ctx) }()
		select {
		case <-ctx.Done():
			return nil
		case err := <-errCh:
			return err
		}
	default:
		return fmt.Errorf("unknown component %q", component)
	}
}
