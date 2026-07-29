package tui

import (
	"strings"
	"testing"

	"embedding_benchmark/internal/bench"
)

func TestRenderCompletionSingle_ShowsLogInfo(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"Provider"})
	m.addCompletionResult(0, &bench.CompletionReport{
		TotalRequests:    3,
		SuccessCount:     3,
		WallTime:         1e9, // 1s in nanoseconds
		LogRequestsFile:  "bench_requests_20260729-153045.jsonl",
		LogResponsesFile: "bench_responses_20260729-153045.jsonl",
		LogDroppedCount:  12,
		LogError:         "disk full",
		Valid:            true,
	})
	var sb strings.Builder
	m.renderCompletionSingle(&sb)
	out := sb.String()
	if !strings.Contains(out, "bench_requests_20260729-153045.jsonl") {
		t.Error("results should show the requests log file path")
	}
	if !strings.Contains(out, "bench_responses_20260729-153045.jsonl") {
		t.Error("results should show the responses log file path")
	}
	if !strings.Contains(out, "12 log entries dropped") {
		t.Error("results should warn about dropped log entries")
	}
	if !strings.Contains(out, "disk full") {
		t.Error("results should show the log write error")
	}
}

func TestRenderCompletionSingle_NoLogInfoWhenDisabled(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"Provider"})
	m.addCompletionResult(0, &bench.CompletionReport{
		TotalRequests: 1,
		SuccessCount:  1,
		WallTime:      1e9,
		Valid:         true,
	})
	var sb strings.Builder
	m.renderCompletionSingle(&sb)
	out := sb.String()
	if strings.Contains(out, "Logs:") {
		t.Error("results should not show log paths when logging disabled")
	}
}
