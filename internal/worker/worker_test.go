package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/onsi/gomega"
)

func TestGivenNoIndexWhenRunningCompleteModeThenWorkerOmitsCompletionIndex(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	args := []string{"--mode=complete", "--duration=0"}

	// when
	code, records, stderr := runTest(t, context.Background(), args, "")

	// then
	assert.Expect(code).To(gomega.Equal(ExitSuccess))
	assert.Expect(stderr).To(gomega.BeEmpty())
	assertOutcomes(t, records, "started", "succeeded")
	assert.Expect(records[0].CompletionIndex).To(gomega.BeNil())
	assert.Expect(records[1].CompletionIndex).To(gomega.BeNil())
}

func TestGivenSelectedFailureIndexWhenRunningThenWorkerFailsWithCompletionIndex(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	args := []string{"--mode=complete", "--duration=0", "--fail-indexes=1,3"}

	// when
	code, records, _ := runTest(t, context.Background(), args, "3")

	// then
	assert.Expect(code).To(gomega.Equal(ExitFailure))
	assertOutcomes(t, records, "started", "failed")
	assert.Expect(records[1].CompletionIndex).NotTo(gomega.BeNil())
	assert.Expect(*records[1].CompletionIndex).To(gomega.Equal(3))
}

func TestGivenCancelledContextWhenRunningWaitModeThenWorkerTerminates(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// when
	code, records, _ := runTest(t, ctx, nil, "0")

	// then
	assert.Expect(code).To(gomega.Equal(ExitTermination))
	assertOutcomes(t, records, "started", "terminated")
}

func TestGivenStartupDelayWhenRunningThenWorkerWaitsBeforeCompleting(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	delay := 15 * time.Millisecond
	started := time.Now()

	// when
	code, _, _ := runTest(t, context.Background(), []string{
		"--mode=complete", "--duration=0", "--startup-delay=" + delay.String(),
	}, "")
	elapsed := time.Since(started)

	// then
	assert.Expect(code).To(gomega.Equal(ExitSuccess))
	assert.Expect(elapsed).To(gomega.BeNumerically(">=", delay))
}

func TestGivenInvalidModeWhenRunningThenValidationFailsBeforeRecordsAreWritten(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	args := []string{"--mode=unknown"}

	// when
	code, records, stderr := runTest(t, context.Background(), args, "")

	// then
	assert.Expect(code).To(gomega.Equal(ExitValidation))
	assert.Expect(records).To(gomega.BeEmpty())
	assert.Expect(stderr).To(gomega.ContainSubstring("use complete or wait"))
}

func TestGivenSuccessfulRunWhenWritingOutputThenRecordsAreNewlineDelimitedJSON(t *testing.T) {
	assert := gomega.NewWithT(t)

	// given
	var stdout bytes.Buffer

	// when
	code := Run(context.Background(), []string{
		"--mode=complete", "--duration=0",
	}, func(string) string { return "2" }, &stdout, &bytes.Buffer{})
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")

	// then
	assert.Expect(code).To(gomega.Equal(ExitSuccess))
	assert.Expect(stdout.String()).To(gomega.HaveSuffix("\n"))
	assert.Expect(lines).To(gomega.HaveLen(2))
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		assert.Expect(json.Valid([]byte(line))).To(gomega.BeTrue(), "invalid JSON line %q", line)
	}
}

func TestGivenInvalidWorkerConfigurationWhenParsingThenValidationFails(t *testing.T) {
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
			assert := gomega.NewWithT(t)

			// given
			args := test.args
			index := test.index

			// when
			_, err := ParseConfig(args, func(string) string { return index })

			// then
			assert.Expect(err).To(gomega.HaveOccurred())
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
	assert := gomega.NewWithT(t)
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, func(string) string { return index }, &stdout, &stderr)
	records := make([]Record, 0, 2)
	decoder := json.NewDecoder(&stdout)
	for decoder.More() {
		var record Record
		assert.Expect(decoder.Decode(&record)).To(gomega.Succeed())
		records = append(records, record)
	}
	return code, records, stderr.String()
}

func assertOutcomes(t *testing.T, records []Record, outcomes ...string) {
	t.Helper()
	assert := gomega.NewWithT(t)
	assert.Expect(records).To(gomega.HaveLen(len(outcomes)))
	for index, outcome := range outcomes {
		assert.Expect(records[index].Outcome).To(gomega.Equal(outcome))
	}
}
