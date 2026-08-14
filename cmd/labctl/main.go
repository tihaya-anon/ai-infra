package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tihaya-anon/ai-infra-lab/internal/lab"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "labctl:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("command required: benchmark, e2e, exercise, or collect")
	}
	switch args[0] {
	case "benchmark":
		return runBenchmark(ctx, args[1:])
	case "exercise":
		return runExercise(ctx, args[1:])
	case "collect":
		return runCollect(ctx, args[1:])
	case "e2e":
		return runE2E(ctx, args[1:])
	case "validate-results":
		return runValidateResults(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runValidateResults(args []string) error {
	flags := flag.NewFlagSet("validate-results", flag.ContinueOnError)
	directory := flags.String("dir", "out/benchmark", "benchmark result directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	paths, err := filepath.Glob(filepath.Join(*directory, "*.json"))
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no benchmark JSON files found in %s", *directory)
	}
	for _, path := range paths {
		if err := lab.ValidateResultFile(path); err != nil {
			return err
		}
	}
	return nil
}

func runE2E(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("e2e", flag.ContinueOnError)
	clusterName := flags.String("cluster", "", "required explicit kind cluster name")
	namespace := flags.String("namespace", "default", "workload namespace")
	output := flags.String("output", "out/e2e", "diagnostic evidence directory")
	timeout := flags.Duration("timeout", 3*time.Minute, "per-scenario timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *clusterName == "" {
		return fmt.Errorf("--cluster is required")
	}
	if err := lab.ValidateKindContext(*clusterName); err != nil {
		return err
	}
	cluster, err := lab.NewCluster()
	if err != nil {
		return err
	}
	runner, err := lab.NewE2ERunner(cluster, lab.E2EOptions{
		Namespace: *namespace, OutputDir: *output, Timeout: *timeout,
	})
	if err != nil {
		return err
	}
	return runner.Run(ctx)
}

func runExercise(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("exercise", flag.ContinueOnError)
	kind := flags.String("kind", "", "capacity, worker-failure, or controller-restart")
	clusterName := flags.String("cluster", "ai-infra-lab", "explicit kind cluster name")
	namespace := flags.String("namespace", "default", "workload namespace")
	output := flags.String("output", "out/evidence", "evidence directory")
	timeout := flags.Duration("timeout", 2*time.Minute, "exercise timeout")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := lab.ValidateKindContext(*clusterName); err != nil {
		return err
	}
	cluster, err := lab.NewCluster()
	if err != nil {
		return err
	}
	runner, err := lab.NewExerciseRunner(cluster, lab.ExerciseOptions{
		Kind: *kind, Namespace: *namespace, OutputDir: *output, Timeout: *timeout,
	})
	if err != nil {
		return err
	}
	return runner.Run(ctx)
}

func runCollect(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("collect", flag.ContinueOnError)
	runID := flags.String("run-id", "", "required run ID label")
	experiment := flags.String("experiment", "manual", "experiment category")
	namespace := flags.String("namespace", "default", "workload namespace")
	output := flags.String("output", "out/evidence", "evidence directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cluster, err := lab.NewCluster()
	if err != nil {
		return err
	}
	collector, err := lab.NewEvidenceCollector(cluster, lab.EvidenceOptions{
		Namespace: *namespace, RunID: *runID, Experiment: *experiment, OutputDir: *output,
	})
	if err != nil {
		return err
	}
	_, err = collector.Collect(ctx)
	return err
}

func runBenchmark(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	clusterName := flags.String("cluster", "ai-infra-lab", "explicit kind cluster name")
	namespace := flags.String("namespace", "default", "workload namespace")
	output := flags.String("output", "out/benchmark", "result directory")
	timeout := flags.Duration("timeout", 2*time.Minute, "per-observation timeout")
	repetitions := flags.Int("repetitions", 1, "runs per profile")
	baseline := flags.String(
		"baseline-config", "deploy/scheduler-profiles/baseline.yaml", "baseline ConfigMap",
	)
	optimized := flags.String(
		"optimized-config", "deploy/scheduler-profiles/optimized.yaml", "optimized ConfigMap",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if err := lab.ValidateKindContext(*clusterName); err != nil {
		return err
	}
	cluster, err := lab.NewCluster()
	if err != nil {
		return err
	}
	runner, err := lab.NewBenchmarkRunner(cluster, lab.BenchmarkOptions{
		Namespace: *namespace, OutputDir: *output, Timeout: *timeout,
		Repetitions: *repetitions,
		Profiles: []lab.Profile{
			{Name: "baseline", ConfigPath: *baseline, ProbeShouldFit: false},
			{Name: "optimized", ConfigPath: *optimized, ProbeShouldFit: true},
		},
	})
	if err != nil {
		return err
	}
	return runner.Run(ctx)
}
