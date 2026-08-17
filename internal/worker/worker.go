// Package worker implements the deterministic process used by the lab workloads.
package worker

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ExitSuccess     = 0
	ExitValidation  = 2
	ExitFailure     = 10
	ExitTermination = 3
)

const (
	ModeComplete = "complete"
	ModeWait     = "wait"
)

// Config is the validated worker runtime configuration.
type Config struct {
	Mode         string
	Duration     time.Duration
	StartupDelay time.Duration
	FailIndexes  map[int]struct{}
	Index        *int
}

// Record is one newline-delimited lifecycle observation.
type Record struct {
	Type            string           `json:"type"`
	Timestamp       string           `json:"timestamp"`
	Mode            string           `json:"mode"`
	CompletionIndex *int             `json:"completionIndex,omitempty"`
	Duration        string           `json:"duration"`
	Outcome         string           `json:"outcome"`
	Parameters      ParameterSummary `json:"parameters"`
	ExitReason      string           `json:"exitReason,omitempty"`
	ExitCode        *int             `json:"exitCode,omitempty"`
}

// ParameterSummary is the bounded, normalized worker input recorded for diagnostics.
type ParameterSummary struct {
	Mode         string `json:"mode"`
	Duration     string `json:"duration"`
	StartupDelay string `json:"startupDelay"`
	FailIndexes  []int  `json:"failIndexes,omitempty"`
}

// Run validates args, executes the selected mode, and returns a stable exit code.
func Run(
	ctx context.Context,
	args []string,
	getenv func(string) string,
	stdout io.Writer,
	stderr io.Writer,
) int {
	config, err := ParseConfig(args, getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "worker: %v\n", err)
		return ExitValidation
	}

	encoder := json.NewEncoder(stdout)
	writeRecord(encoder, config, "start", "started", nil)
	if !wait(ctx, config.StartupDelay) {
		writeRecord(encoder, config, "result", "terminated", intPtr(ExitTermination))
		return ExitTermination
	}

	switch config.Mode {
	case ModeWait:
		<-ctx.Done()
		writeRecord(encoder, config, "result", "terminated", intPtr(ExitTermination))
		return ExitTermination
	case ModeComplete:
		if !wait(ctx, config.Duration) {
			writeRecord(encoder, config, "result", "terminated", intPtr(ExitTermination))
			return ExitTermination
		}
		if config.Index != nil {
			if _, selected := config.FailIndexes[*config.Index]; selected {
				writeRecord(encoder, config, "result", "failed", intPtr(ExitFailure))
				return ExitFailure
			}
		}
		writeRecord(encoder, config, "result", "succeeded", intPtr(ExitSuccess))
		return ExitSuccess
	default:
		panic("validated worker mode is not implemented")
	}
}

// ParseConfig parses flags and the Indexed Job completion identity.
func ParseConfig(args []string, getenv func(string) string) (Config, error) {
	flags := flag.NewFlagSet("worker", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mode := flags.String("mode", ModeWait, "complete or wait")
	duration := flags.Duration("duration", time.Second, "bounded completion duration")
	startupDelay := flags.Duration("startup-delay", 0, "delay before workload execution")
	failIndexes := flags.String("fail-indexes", "", "comma-separated completion indexes")
	if err := flags.Parse(args); err != nil {
		return Config{}, fmt.Errorf("invalid flags: %w", err)
	}
	if flags.NArg() != 0 {
		return Config{}, fmt.Errorf(
			"unexpected positional arguments: %s", strings.Join(flags.Args(), " "),
		)
	}
	if *mode != ModeComplete && *mode != ModeWait {
		return Config{}, fmt.Errorf("invalid --mode %q: use complete or wait", *mode)
	}
	if *duration < 0 {
		return Config{}, errors.New("--duration must not be negative")
	}
	if *startupDelay < 0 {
		return Config{}, errors.New("--startup-delay must not be negative")
	}

	selected, err := parseIndexes(*failIndexes)
	if err != nil {
		return Config{}, err
	}
	index, err := parseIndex(getenv("JOB_COMPLETION_INDEX"))
	if err != nil {
		return Config{}, err
	}
	return Config{
		Mode: *mode, Duration: *duration, StartupDelay: *startupDelay,
		FailIndexes: selected, Index: index,
	}, nil
}

func parseIndexes(value string) (map[int]struct{}, error) {
	indexes := make(map[int]struct{})
	if value == "" {
		return indexes, nil
	}
	for _, item := range strings.Split(value, ",") {
		index, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || index < 0 {
			return nil, fmt.Errorf("invalid --fail-indexes value %q", item)
		}
		indexes[index] = struct{}{}
	}
	return indexes, nil
}

func parseIndex(value string) (*int, error) {
	if value == "" {
		return nil, nil
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 {
		return nil, fmt.Errorf("invalid JOB_COMPLETION_INDEX %q", value)
	}
	return &index, nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	if duration == 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func writeRecord(
	encoder *json.Encoder,
	config Config,
	recordType, outcome string,
	exitCode *int,
) {
	failIndexes := make([]int, 0, len(config.FailIndexes))
	for index := range config.FailIndexes {
		failIndexes = append(failIndexes, index)
	}
	sort.Ints(failIndexes)
	exitReason := ""
	if recordType == "result" {
		exitReason = outcome
	}
	_ = encoder.Encode(Record{
		Type: recordType, Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Mode: config.Mode, CompletionIndex: config.Index,
		Duration: config.Duration.String(), Outcome: outcome,
		Parameters: ParameterSummary{
			Mode: config.Mode, Duration: config.Duration.String(),
			StartupDelay: config.StartupDelay.String(), FailIndexes: failIndexes,
		},
		ExitReason: exitReason, ExitCode: exitCode,
	})
}

func intPtr(value int) *int { return &value }
