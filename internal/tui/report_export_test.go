package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"embedding_benchmark/internal/bench"
)

func TestBenchDoneMsgWritesMarkdownReport(t *testing.T) {
	dir := t.TempDir()
	old := markdownReportDir
	markdownReportDir = dir
	defer func() { markdownReportDir = old }()

	m := NewModel(nil)
	m.apiMode = bench.ModeCompletion
	m.testMode = "single"
	m.providers = []bench.ProviderConfig{{Name: "p", URL: "http://x", Model: "m"}}
	m.cfg = bench.BenchConfig{Mode: bench.ModeCompletion, Concurrency: 1, TotalRequests: 1}

	updated, _ := m.Update(BenchDoneMsg{
		ProviderIndex:    0,
		CompletionReport: &bench.CompletionReport{TotalRequests: 1, SuccessCount: 1, Valid: true},
	})
	um := updated.(Model)

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.HasPrefix(files[0].Name(), "bench_report_") || !strings.HasSuffix(files[0].Name(), ".md") {
		t.Fatalf("expected one bench_report_*.md file, got %v", files)
	}
	data, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if !strings.Contains(string(data), "# Benchmark Report — Chat Completion") {
		t.Error("report file missing title")
	}
	if um.results.reportFile != files[0].Name() {
		t.Errorf("results.reportFile = %q, want %q", um.results.reportFile, files[0].Name())
	}
	if um.screen != ScreenResults {
		t.Error("should still transition to results screen")
	}
}

func TestBenchDoneMsgPKWritesSingleReport(t *testing.T) {
	dir := t.TempDir()
	old := markdownReportDir
	markdownReportDir = dir
	defer func() { markdownReportDir = old }()

	m := NewModel(nil)
	m.apiMode = bench.ModeEmbedding
	m.testMode = "pk"
	m.providers = []bench.ProviderConfig{
		{Name: "A", URL: "http://a", Model: "m"},
		{Name: "B", URL: "http://b", Model: "m"},
	}
	m.cfg = bench.BenchConfig{Mode: bench.ModeEmbedding, Concurrency: 1, TotalRequests: 1}

	r := &bench.EmbeddingReport{TotalRequests: 1, SuccessCount: 1, Valid: true}
	updated, _ := m.Update(BenchDoneMsg{ProviderIndex: 0, EmbeddingReport: r})
	um := updated.(Model)
	if um.results.reportFile != "" {
		t.Error("report must not be written before all providers finish")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("no file expected before all providers finish")
	}

	updated, _ = um.Update(BenchDoneMsg{ProviderIndex: 1, EmbeddingReport: r})
	um = updated.(Model)
	files, _ = os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("expected exactly one combined PK report file, got %v", files)
	}
	data, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if !strings.Contains(string(data), "(PK)") {
		t.Error("PK report missing (PK) title")
	}
}
