package tui

import (
	"testing"

	tiktoken "github.com/pkoukk/tiktoken-go"

	"embedding_benchmark/internal/bench"
)

func testTokenizer(t *testing.T) *tiktoken.Tiktoken {
	t.Helper()
	tkm, err := bench.InitTiktoken("")
	if err != nil {
		t.Fatalf("InitTiktoken: %v", err)
	}
	return tkm
}

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

func TestPrepareBenchTextSameAndUnique(t *testing.T) {
	tkm := testTokenizer(t)

	same, tokens, err := prepareBenchText(bench.BenchConfig{TargetTokens: 50, TotalRequests: 3}, tkm)
	if err != nil {
		t.Fatalf("same: %v", err)
	}
	if len(same) != 1 {
		t.Fatalf("same texts len = %d, want 1", len(same))
	}
	if tokens != 50 {
		t.Fatalf("same actualTokens = %d, want 50", tokens)
	}

	unique, tokens, err := prepareBenchText(bench.BenchConfig{
		TargetTokens: 50, TotalRequests: 3, UniqueInputs: true,
	}, tkm)
	if err != nil {
		t.Fatalf("unique: %v", err)
	}
	if len(unique) != 3 {
		t.Fatalf("unique texts len = %d, want 3", len(unique))
	}
	if tokens != 50 {
		t.Fatalf("unique actualTokens = %d, want 50", tokens)
	}
	if unique[0] == unique[1] || unique[1] == unique[2] {
		t.Fatalf("unique texts should differ: %q / %q / %q", unique[0], unique[1], unique[2])
	}
}
