package tui

import (
	"testing"

	"embedding_benchmark/internal/bench"
)

func TestRunningModelAccumulatesProgressErrors(t *testing.T) {
	model := newRunningModel([]bench.ProviderConfig{{Name: "Provider"}}, bench.BenchConfig{TotalRequests: 2})

	updated, _ := model.update(ProgressMsg{
		ProviderIndex: 0,
		Completed:     0,
		TotalErrors:   1,
		ErrorDetail:   "HTTP 429: rate limited",
		ErrorCategory: bench.ErrorCategoryRateLimit,
	})

	if !updated.hasErrors {
		t.Fatalf("hasErrors = false, want true")
	}
	if got := updated.errorDetails["HTTP 429: rate limited"]; got != 1 {
		t.Fatalf("error detail count = %d, want 1", got)
	}
	if got := updated.errorCategories[bench.ErrorCategoryRateLimit]; got != 1 {
		t.Fatalf("error category count = %d, want 1", got)
	}
}
