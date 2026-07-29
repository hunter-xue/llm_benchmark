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

func fullCompletionReportForMD() *CompletionReport {
	return &CompletionReport{
		TotalRequests: 100, SuccessCount: 98, ErrorCount: 2,
		ErrorCategories: map[string]int{"timeout": 1, "rate_limit": 1},
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
		LogDroppedCount:  7,
		LogError:         "disk full",
	}
}

func TestMarkdownBenchReport_CompletionSingle(t *testing.T) {
	providers := []ProviderConfig{{Name: "p1", URL: "http://x/v1/chat/completions", APIKey: "sk-abcdefgh", Model: "qwen3", CustomParams: `{"temperature":0}`}}
	cfg := BenchConfig{
		Mode: ModeCompletion, Concurrency: 10, TotalRequests: 100, TargetTokens: 1000,
		MaxOutputTokens: 512, SystemPrompt: "You are helpful",
		LoadModel: LoadModelClosedLoop, TTFTIncludesReasoning: true, RequestLogging: true,
	}
	md := MarkdownBenchReport(ModeCompletion, "single", providers, cfg, nil, []*CompletionReport{fullCompletionReportForMD()}, time.Now())

	for _, want := range []string{
		"# Benchmark Report — Chat Completion",
		"| Custom Params | {\"temperature\":0} |",
		"| Max Output Tokens | 512 |",
		"| Load Model | closed_loop |",
		"| TTFT Includes Reasoning | true |",
		"| Request Logging | true |",
		"| System Prompt | You are helpful |",
		"| Output TPS | 1234.0 |",
		"| Avg Output Tokens | 128.5 |",
		"| API Prompt Tokens | 49000 |",
		"| API Completion Tokens | 12583 |",
		"| API Total Tokens | 61583 |",
		"| API Usage Samples | 98 / 98 |",
		"| Missing API Usage | 2 |",
		"| TTFT Avg | 100.00 ms |",
		"| TTFT P99 | 200.00 ms |",
		"| TPOT Avg | 10.00 ms/tok |",
		"| E2E P99 | 2000.00 ms |",
		"| Skipped SSE Chunks | 3 (warning) |",
		"| Queue Time Avg | 5.00 ms |",
		"| Queue Time P99 | 12.00 ms |",
		"## Logs",
		"- Requests: `bench_requests_20260729-120000.jsonl`",
		"- Responses: `bench_responses_20260729-120000.jsonl`",
		"- ⚠ 7 log entries dropped (disk too slow)",
		"- ⚠ log write error: disk full",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q\n--- report ---\n%s", want, md)
		}
	}
}

func TestMarkdownBenchReport_CompletionSingleMinimal(t *testing.T) {
	providers := []ProviderConfig{{Name: "p1", URL: "http://x", Model: "m"}}
	cfg := BenchConfig{Mode: ModeCompletion, Concurrency: 1, TotalRequests: 1, LoadModel: LoadModelClosedLoop}
	r := &CompletionReport{TotalRequests: 1, SuccessCount: 1, Valid: true}
	md := MarkdownBenchReport(ModeCompletion, "single", providers, cfg, nil, []*CompletionReport{r}, time.Now())

	for _, notWant := range []string{"Queue Time", "## Logs", "Skipped SSE Chunks", "Missing API Usage", "System Prompt", "Custom Params"} {
		if strings.Contains(md, notWant) {
			t.Errorf("minimal report should not contain %q", notWant)
		}
	}
	for _, want := range []string{"| Max Output Tokens | unlimited |", "| API Prompt Tokens | N/A |", "| API Usage Samples | N/A / 1 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

func TestMarkdownBenchReport_OpenLoopParams(t *testing.T) {
	providers := []ProviderConfig{{Name: "p1", URL: "http://x", Model: "m"}}
	cfg := BenchConfig{
		Mode: ModeCompletion, TotalRequests: 50, TargetTokens: 100,
		LoadModel: LoadModelOpenLoop, RequestRate: 20, MaxInFlight: 5, Concurrency: 5,
	}
	r := &CompletionReport{TotalRequests: 50, SuccessCount: 50, Valid: true}
	md := MarkdownBenchReport(ModeCompletion, "single", providers, cfg, nil, []*CompletionReport{r}, time.Now())
	for _, want := range []string{"| Load Model | open_loop |", "| Request Rate | 20 req/s |", "| Max In-Flight | 5 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("open-loop report missing %q", want)
		}
	}
	if strings.Contains(md, "| Concurrency |") {
		t.Error("open-loop report should replace Concurrency with Request Rate / Max In-Flight")
	}
}

func TestMarkdownBenchReport_CompletionAllFailed(t *testing.T) {
	providers := []ProviderConfig{{Name: "p1", URL: "http://x", Model: "m"}}
	cfg := BenchConfig{Mode: ModeCompletion, Concurrency: 1, TotalRequests: 3, RequestLogging: true}
	r := &CompletionReport{
		TotalRequests: 3, ErrorCount: 3, Valid: false,
		LogRequestsFile:  "bench_requests_x.jsonl",
		LogResponsesFile: "bench_responses_x.jsonl",
	}
	md := MarkdownBenchReport(ModeCompletion, "single", providers, cfg, nil, []*CompletionReport{r}, time.Now())
	if !strings.Contains(md, "_All requests failed — no metrics available._") {
		t.Error("expected all-failed note")
	}
	if strings.Contains(md, "| TTFT Avg |") {
		t.Error("metric sections must be omitted when all requests failed")
	}
	if !strings.Contains(md, "- Requests: `bench_requests_x.jsonl`") {
		t.Error("Logs section must still be present when all requests failed")
	}
}

func TestMarkdownBenchReport_PipeEscaped(t *testing.T) {
	providers := []ProviderConfig{{Name: "p1", URL: "http://x", Model: "m", CustomParams: `{"a":"b|c"}`}}
	cfg := BenchConfig{Mode: ModeEmbedding, Concurrency: 1, TotalRequests: 1}
	r := &EmbeddingReport{TotalRequests: 1, SuccessCount: 1, Valid: true}
	md := MarkdownBenchReport(ModeEmbedding, "single", providers, cfg, []*EmbeddingReport{r}, nil, time.Now())
	if !strings.Contains(md, `{"a":"b\|c"}`) {
		t.Error("pipe characters in values must be escaped")
	}
}
