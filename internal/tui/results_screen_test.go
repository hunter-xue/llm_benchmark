package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"embedding_benchmark/internal/bench"
)

// fullCompletionReport returns a report populated with every section the
// results view can render (logging, errors, skipped chunks, queue time).
func fullCompletionReport() *bench.CompletionReport {
	return &bench.CompletionReport{
		TotalRequests: 100, SuccessCount: 98, ErrorCount: 2,
		ErrorCategories: map[string]int{"timeout": 1, "rate_limit": 1},
		ErrorDetails:    map[string]int{"x": 2},
		Valid:           true,
		WallTime:        10 * time.Second,
		RPS:             9.8, InputTPS: 4900, OutputTPS: 1234,
		InputTPM: 294000, OutputTPM: 74040, AvgOutputTokens: 128.5,
		APIPromptTokens: 49000, APICompletionTokens: 12583, APITotalTokens: 61583,
		APIUsageCount: 98, MissingAPIUsageCount: 2,
		TTFTAvg: 100, TTFTp50: 90, TTFTp90: 150, TTFTp99: 200,
		TPOTAvg: 10, TPOTp50: 9, TPOTp90: 15, TPOTp99: 20,
		E2EAvg: 1300, E2Ep50: 1200, E2Ep90: 1800, E2Ep99: 2000,
		SkippedChunks:    3,
		QueueTimeAvg:     5, QueueTimeP50: 4, QueueTimeP90: 8, QueueTimeP99: 12,
		LogRequestsFile:  "bench_requests_20260729-120000.jsonl",
		LogResponsesFile: "bench_responses_20260729-120000.jsonl",
	}
}

func TestResultsViewFitsTerminalHeight(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"p"})
	m.addCompletionResult(0, fullCompletionReport())
	m.setSize(100, 24)

	out := m.view(100, 24)
	if n := strings.Count(out, "\n") + 1; n > 24 {
		t.Errorf("results view rendered %d lines, exceeds terminal height 24", n)
	}
}

func TestResultsViewScrollsWithKeys(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"p"})
	m.addCompletionResult(0, fullCompletionReport())
	m.setSize(100, 24)

	top := m.view(100, 24)
	if !strings.Contains(top, "Total Requests") {
		t.Fatal("initial view should show the summary rows at the top")
	}

	for i := 0; i < 10; i++ {
		m, _ = m.update(tea.KeyMsg{Type: tea.KeyPgDown})
	}
	bottom := m.view(100, 24)
	if !strings.Contains(bottom, "E2E P99") {
		t.Error("scrolled-to-bottom view should show the E2E section")
	}
	if strings.Contains(bottom, "Total Requests") {
		t.Error("scrolled-to-bottom view should no longer show the top summary rows")
	}
}

func TestResultsPlainTextContainsAllSections(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"p"})
	m.addCompletionResult(0, fullCompletionReport())
	m.setSize(100, 24)

	plain := m.plainText()
	for _, want := range []string{"Total Requests", "TTFT Avg", "TPOT P99", "E2E P99", "Queue Time P99", "ctrl+e export"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plainText() missing %q (export must contain full untruncated content)", want)
		}
	}
}

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

func TestRenderCompletionSingle_ShowsLogInfoWhenAllRequestsFailed(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"Provider"})
	m.addCompletionResult(0, &bench.CompletionReport{
		TotalRequests:    3,
		SuccessCount:     0,
		ErrorCount:       3,
		LogRequestsFile:  "bench_requests_20260729-153045.jsonl",
		LogResponsesFile: "bench_responses_20260729-153045.jsonl",
		ErrorDetails:     map[string]int{"HTTP 500": 3},
		Valid:            false,
	})
	var sb strings.Builder
	m.renderCompletionSingle(&sb)
	out := sb.String()
	if !strings.Contains(out, "bench_requests_20260729-153045.jsonl") {
		t.Error("results should show the requests log file path even when all requests failed")
	}
	if !strings.Contains(out, "bench_responses_20260729-153045.jsonl") {
		t.Error("results should show the responses log file path even when all requests failed")
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
