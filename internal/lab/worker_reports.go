package lab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/tihaya-anon/ai-infra-lab/internal/topology"
	"github.com/tihaya-anon/ai-infra-lab/internal/worker"
	corev1 "k8s.io/api/core/v1"
)

func workerRuntimeReports(
	pods []corev1.Pod,
	logs map[string][]byte,
) ([]WorkerRuntime, error) {
	reports := make([]WorkerRuntime, 0, len(pods))
	var parseErrors []error
	for _, pod := range pods {
		report := WorkerRuntime{
			Pod: pod.Name, Workload: pod.Labels[topology.JobLabel],
			CompletionIndex: pod.Labels["batch.kubernetes.io/job-completion-index"],
		}
		applyContainerState(&report, pod.Status.ContainerStatuses)
		if data, exists := logs[pod.Name]; exists {
			if err := applyWorkerRecords(&report, data); err != nil {
				parseErrors = append(parseErrors, fmt.Errorf("parse worker log %s: %w", pod.Name, err))
			}
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Pod < reports[j].Pod })
	return reports, errors.Join(parseErrors...)
}

func applyWorkerRecords(report *WorkerRuntime, data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		record := worker.Record{}
		if err := decoder.Decode(&record); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		timestamp, err := time.Parse(time.RFC3339Nano, record.Timestamp)
		if err != nil {
			return fmt.Errorf("parse timestamp %q: %w", record.Timestamp, err)
		}
		report.Parameters = WorkerParameters{
			Mode: record.Parameters.Mode, Duration: record.Parameters.Duration,
			StartupDelay: record.Parameters.StartupDelay,
			FailIndexes:  append([]int(nil), record.Parameters.FailIndexes...),
		}
		switch record.Type {
		case "start":
			report.StartedAt = timePtr(timestamp)
		case "result":
			report.FinishedAt = timePtr(timestamp)
			report.ExitReason = record.ExitReason
			if record.ExitCode != nil {
				report.ExitCode = intPtr(*record.ExitCode)
			}
		}
	}
}

func applyContainerState(report *WorkerRuntime, statuses []corev1.ContainerStatus) {
	for _, status := range statuses {
		if status.Name != "worker" {
			continue
		}
		if status.State.Running != nil && report.StartedAt == nil {
			report.StartedAt = timePtr(status.State.Running.StartedAt.Time)
		}
		if status.State.Terminated == nil {
			continue
		}
		terminated := status.State.Terminated
		if report.StartedAt == nil {
			report.StartedAt = timePtr(terminated.StartedAt.Time)
		}
		report.FinishedAt = timePtr(terminated.FinishedAt.Time)
		report.ExitReason = terminated.Reason
		report.ExitCode = intPtr(int(terminated.ExitCode))
	}
}

func timePtr(value time.Time) *time.Time { return &value }

func intPtr(value int) *int { return &value }
