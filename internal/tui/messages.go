package tui

import "embedding_benchmark/internal/bench"

// ProgressMsg is sent from benchmark goroutines to report incremental progress.
type ProgressMsg struct {
	ProviderIndex int // 0 for single/first provider, 1 for second provider in PK mode
	Completed     int
	TotalErrors   int
}

// BenchDoneMsg is sent when a benchmark goroutine finishes.
type BenchDoneMsg struct {
	ProviderIndex    int
	EmbeddingReport  *bench.EmbeddingReport  // non-nil in embedding mode
	CompletionReport *bench.CompletionReport // non-nil in completion mode
}

// CompareResponseMsg carries one provider's raw JSON response for the comparison screen.
type CompareResponseMsg struct {
	ProviderIndex int
	Headers       string // formatted HTTP response headers
	Body          string // pretty-printed JSON on success
	Err           error
}

// SingleResponseMsg carries the raw JSON response for the single response view screen.
type SingleResponseMsg struct {
	Headers string // formatted HTTP response headers
	Body    string // pretty-printed JSON on success
	Err     error
}
