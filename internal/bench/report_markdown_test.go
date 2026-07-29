package bench

import (
	"strings"
	"testing"
	"time"
)

func TestMarkdownBenchReport_EmbeddingSingle(t *testing.T) {
	providers := []ProviderConfig{{Name: "p1", URL: "http://x/v1/embeddings", APIKey: "sk-abcdefgh", Model: "bge-m3"}}
	cfg := BenchConfig{Mode: ModeEmbedding, Concurrency: 5, TotalRequests: 10, TargetTokens: 100}
	report := &EmbeddingReport{
		TotalRequests: 10, SuccessCount: 9, ErrorCount: 1,
		ErrorCategories: map[string]int{"timeout": 1},
		WallTime:        2 * time.Second,
		RPS:             4.5, InputTPS: 450, InputTPM: 27000,
		LatencyAvg: 100, LatencyP50: 90, LatencyP90: 150, LatencyP99: 200,
		Valid: true,
	}
	now := time.Date(2026, 7, 29, 15, 30, 45, 0, time.Local)
	md := MarkdownBenchReport(ModeEmbedding, "single", providers, cfg, []*EmbeddingReport{report}, nil, now)

	for _, want := range []string{
		"# Benchmark Report — Embedding",
		"Generated: 2026-07-29 15:30:45",
		"## Test Parameters",
		"| Parameter | Value |",
		"| Provider | p1 |",
		"| URL | http://x/v1/embeddings |",
		"| Model | bge-m3 |",
		"| API Key | ***efgh |",
		"| API Mode | embedding |",
		"| Test Mode | single |",
		"| Concurrency | 5 |",
		"| Total Requests | 10 |",
		"| Input Tokens | 100 |",
		"## Results",
		"| Metric | Value |",
		"| Total Requests | 10 |",
		"| Successful | 9 (90.0%) |",
		"| Failed | 1 |",
		"| Error Categories | timeout: 1 |",
		"| Wall Time | 2.00 s |",
		"| RPS | 4.50 |",
		"| Input TPS | 450.0 |",
		"| Input TPM | 27000 |",
		"| Latency Avg | 100.00 ms |",
		"| Latency P99 | 200.00 ms |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q\n--- report ---\n%s", want, md)
		}
	}
	for _, notWant := range []string{"Max Output Tokens", "TTFT Includes Reasoning", "Request Logging", "Load Model"} {
		if strings.Contains(md, notWant) {
			t.Errorf("embedding report should not contain %q", notWant)
		}
	}
}

func TestMarkdownBenchReport_EmbeddingSingleAllFailed(t *testing.T) {
	providers := []ProviderConfig{{Name: "p1", URL: "http://x", Model: "m"}}
	cfg := BenchConfig{Mode: ModeEmbedding, Concurrency: 1, TotalRequests: 3}
	report := &EmbeddingReport{
		TotalRequests: 3, ErrorCount: 3,
		ErrorCategories: map[string]int{"server_error": 3},
		Valid: false,
	}
	md := MarkdownBenchReport(ModeEmbedding, "single", providers, cfg, []*EmbeddingReport{report}, nil, time.Now())
	if !strings.Contains(md, "_All requests failed — no metrics available._") {
		t.Error("expected all-failed note")
	}
	if !strings.Contains(md, "| Failed | 3 |") {
		t.Error("summary table must still be present")
	}
	if strings.Contains(md, "| RPS |") {
		t.Error("metric table must be omitted when all requests failed")
	}
}

func TestMdRedactAPIKey(t *testing.T) {
	cases := map[string]string{"": "(none)", "abcd": "***", "sk-abcdefgh": "***efgh"}
	for in, want := range cases {
		if got := mdRedactAPIKey(in); got != want {
			t.Errorf("mdRedactAPIKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMdEscape(t *testing.T) {
	if got := mdEscape("a|b\nc\rd"); got != "a\\|b c d" {
		t.Errorf("mdEscape = %q", got)
	}
}
