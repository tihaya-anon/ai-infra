package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRunCompleteSuccessWithoutIndex(t *testing.T) {
	code, records, stderr := runTest(t, context.Background(), []string{
		"--mode=complete", "--duration=0",
	}, "")
	if code != ExitSuccess || stderr != "" {
		t.Fatalf("got code %d and stderr %q", code, stderr)
	}
	assertOutcomes(t, records, "started", "succeeded")
	if records[0].CompletionIndex != nil || records[1].CompletionIndex != nil {
		t.Fatal("direct execution must report no completion index")
	}
}

func TestRunSelectedIndexFails(t *testing.T) {
	code, records, _ := runTest(t, context.Background(), []string{
		"--mode=complete", "--duration=0", "--fail-indexes=1,3",
	}, "3")
	if code != ExitFailure {
		t.Fatalf("got code %d, want %d", code, ExitFailure)
	}
	assertOutcomes(t, records, "started", "failed")
	if records[1].CompletionIndex == nil || *records[1].CompletionIndex != 3 {
		t.Fatalf("unexpected result index: %#v", records[1].CompletionIndex)
	}
}

func TestRunWaitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	code, records, _ := runTest(t, ctx, nil, "0")
	if code != ExitTermination {
		t.Fatalf("got code %d, want %d", code, ExitTermination)
	}
	assertOutcomes(t, records, "started", "terminated")
}

func TestRunHonorsStartupDelay(t *testing.T) {
	started := time.Now()
	code, _, _ := runTest(t, context.Background(), []string{
		"--mode=complete", "--duration=0", "--startup-delay=15ms",
	}, "")
	if code != ExitSuccess || time.Since(started) < 15*time.Millisecond {
		t.Fatalf("worker returned too early with code %d", code)
	}
}

func TestRunRejectsInvalidFlagsBeforeStart(t *testing.T) {
	code, records, stderr := runTest(t, context.Background(), []string{"--mode=unknown"}, "")
	if code != ExitValidation || len(records) != 0 {
		t.Fatalf("got code %d and records %#v", code, records)
	}
	if !strings.Contains(stderr, "use complete or wait") {
		t.Fatalf("got non-actionable diagnostic %q", stderr)
	}
}

func TestRunOutputIsNewlineDelimitedJSON(t *testing.T) {
	var stdout bytes.Buffer
	code := Run(context.Background(), []string{
		"--mode=complete", "--duration=0",
	}, func(string) string { return "2" }, &stdout, &bytes.Buffer{})
	if code != ExitSuccess || strings.Count(stdout.String(), "\n") != 2 {
		t.Fatalf("got code %d and output %q", code, stdout.String())
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if !json.Valid([]byte(line)) {
			t.Fatalf("invalid JSON line %q", line)
		}
	}
}

func TestParseConfigRejectsInvalidIndexAndDuration(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		index string
	}{
		{name: "negative duration", args: []string{"--duration=-1s"}},
		{name: "bad failure index", args: []string{"--fail-indexes=a"}},
		{name: "bad completion index", index: "worker-a"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseConfig(test.args, func(string) string { return test.index })
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func runTest(
	t *testing.T,
	ctx context.Context,
	args []string,
	index string,
) (int, []Record, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, func(string) string { return index }, &stdout, &stderr)
	records := make([]Record, 0, 2)
	decoder := json.NewDecoder(&stdout)
	for decoder.More() {
		var record Record
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return code, records, stderr.String()
}

func assertOutcomes(t *testing.T, records []Record, outcomes ...string) {
	t.Helper()
	if len(records) != len(outcomes) {
		t.Fatalf("got %d records, want %d: %#v", len(records), len(outcomes), records)
	}
	for index, outcome := range outcomes {
		if records[index].Outcome != outcome {
			t.Fatalf("record %d outcome %q, want %q", index, records[index].Outcome, outcome)
		}
	}
}
