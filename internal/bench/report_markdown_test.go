package bench

import (
	"fmt"
	"os"
	"path/filepath"
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
		Valid:           false,
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
		APIUsageCount: 96, MissingAPIUsageCount: 2,
		TTFTAvg: 100, TTFTp50: 90, TTFTp90: 150, TTFTp99: 200,
		TPOTAvg: 10, TPOTp50: 9, TPOTp90: 15, TPOTp99: 20,
		E2EAvg: 1300, E2Ep50: 1200, E2Ep90: 1800, E2Ep99: 2000,
		SkippedChunks: 3,
		QueueTimeAvg:  5, QueueTimeP50: 4, QueueTimeP90: 8, QueueTimeP99: 12,
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
		"| API Usage Samples | 96 / 98 |",
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

func TestMarkdownBenchReport_CompletionPK(t *testing.T) {
	providers := []ProviderConfig{
		{Name: "Alpha", URL: "http://a", Model: "m1", APIKey: "sk-aaaa1111"},
		{Name: "Beta", URL: "http://b", Model: "m2", APIKey: "sk-bbbb2222"},
	}
	cfg := BenchConfig{Mode: ModeCompletion, Concurrency: 4, TotalRequests: 20, TargetTokens: 100, MaxOutputTokens: 64, LoadModel: LoadModelClosedLoop}
	a := &CompletionReport{
		TotalRequests: 20, SuccessCount: 20, Valid: true,
		RPS: 9.8, InputTPS: 980, OutputTPS: 300, InputTPM: 58800, OutputTPM: 18000,
		APIPromptTokens: 2000, APICompletionTokens: 600, APITotalTokens: 2600, APIUsageCount: 20,
		TTFTAvg: 100, TTFTp50: 90, TTFTp90: 150, TTFTp99: 200,
		TPOTAvg: 10, TPOTp50: 9, TPOTp90: 15, TPOTp99: 20,
		E2EAvg: 1000, E2Ep50: 900, E2Ep90: 1500, E2Ep99: 2000,
	}
	b := &CompletionReport{
		TotalRequests: 20, SuccessCount: 18, ErrorCount: 2, Valid: true,
		RPS: 8.1, InputTPS: 810, OutputTPS: 320, InputTPM: 48600, OutputTPM: 19200,
		APIPromptTokens: 1800, APICompletionTokens: 640, APITotalTokens: 2440, APIUsageCount: 18,
		TTFTAvg: 120, TTFTp50: 110, TTFTp90: 170, TTFTp99: 220,
		TPOTAvg: 9, TPOTp50: 8, TPOTp90: 14, TPOTp99: 19,
		E2EAvg: 1100, E2Ep50: 1000, E2Ep90: 1600, E2Ep99: 2100,
	}
	md := MarkdownBenchReport(ModeCompletion, "pk", providers, cfg, nil, []*CompletionReport{a, b}, time.Now())

	for _, want := range []string{
		"# Benchmark Report — Chat Completion (PK)",
		"| Parameter | Alpha | Beta |",
		"| URL | http://a | http://b |",
		"| API Key | ***1111 | ***2222 |",
		"| Metric | Alpha | Beta |",
		"| Success / Total | 20 / 20 | 18 / 20 |",
		"| API Prompt Tokens | 2000 | 1800 |",
		"| API Usage Samples | 20 / 20 | 18 / 18 |",
		"| RPS | **9.80** | 8.10 |",                // higher wins → Alpha bold
		"| Output TPS | 300.0 | **320.0** |",       // Beta bold
		"| TTFT Avg | **100.00 ms** | 120.00 ms |", // lower wins → Alpha bold
		"| TPOT Avg | 10.00 ms/tok | **9.00 ms/tok** |",
		"| E2E P99 | **2000.00 ms** | 2100.00 ms |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("PK report missing %q\n--- report ---\n%s", want, md)
		}
	}
	// Request Logging toggle does not exist in PK mode
	if strings.Contains(md, "Request Logging") {
		t.Error("PK report should not contain Request Logging row")
	}
	// no queue time data → no queue rows
	if strings.Contains(md, "Queue Time") {
		t.Error("PK report should not contain Queue Time rows when both providers have none")
	}
}

func TestMarkdownBenchReport_EmbeddingPK(t *testing.T) {
	providers := []ProviderConfig{
		{Name: "A", URL: "http://a", Model: "m1"},
		{Name: "B", URL: "http://b", Model: "m2"},
	}
	cfg := BenchConfig{Mode: ModeEmbedding, Concurrency: 2, TotalRequests: 10, TargetTokens: 50}
	a := &EmbeddingReport{TotalRequests: 10, SuccessCount: 10, Valid: true, RPS: 5, LatencyAvg: 100}
	b := &EmbeddingReport{TotalRequests: 10, SuccessCount: 10, Valid: true, RPS: 6, LatencyAvg: 120}
	md := MarkdownBenchReport(ModeEmbedding, "pk", providers, cfg, []*EmbeddingReport{a, b}, nil, time.Now())
	for _, want := range []string{
		"# Benchmark Report — Embedding (PK)",
		"| RPS | 5.00 | **6.00** |",
		"| Latency Avg | **100.00 ms** | 120.00 ms |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("embedding PK report missing %q", want)
		}
	}
}

func TestMarkdownBenchReport_PKInvalidProvider(t *testing.T) {
	providers := []ProviderConfig{
		{Name: "A", URL: "http://a", Model: "m1"},
		{Name: "B", URL: "http://b", Model: "m2"},
	}
	cfg := BenchConfig{Mode: ModeCompletion, Concurrency: 2, TotalRequests: 10, LoadModel: LoadModelClosedLoop}
	a := &CompletionReport{TotalRequests: 10, SuccessCount: 10, Valid: true, RPS: 5, APIUsageCount: 0}
	b := &CompletionReport{TotalRequests: 10, ErrorCount: 10, Valid: false}
	md := MarkdownBenchReport(ModeCompletion, "pk", providers, cfg, nil, []*CompletionReport{a, b}, time.Now())
	if !strings.Contains(md, "_B: all requests failed_") {
		t.Error("expected failure note for provider B")
	}
	if !strings.Contains(md, "| Success / Total | 10 / 10 | 0 / 10 |") {
		t.Error("Success / Total row must still show both providers")
	}
	if strings.Contains(md, "| RPS |") {
		t.Error("metric rows must be omitted when a provider is invalid")
	}
	if !strings.Contains(md, "| API Prompt Tokens | N/A | N/A |") {
		t.Error("API usage rows render N/A when no usage data")
	}
}

func TestMarkdownBenchReport_PKTie(t *testing.T) {
	providers := []ProviderConfig{{Name: "A", URL: "http://a", Model: "m"}, {Name: "B", URL: "http://b", Model: "m"}}
	cfg := BenchConfig{Mode: ModeEmbedding, Concurrency: 1, TotalRequests: 1}
	a := &EmbeddingReport{TotalRequests: 1, SuccessCount: 1, Valid: true, RPS: 5}
	b := &EmbeddingReport{TotalRequests: 1, SuccessCount: 1, Valid: true, RPS: 5}
	md := MarkdownBenchReport(ModeEmbedding, "pk", providers, cfg, []*EmbeddingReport{a, b}, nil, time.Now())
	if strings.Contains(md, "**") {
		t.Error("tie should not bold any cell")
	}
	if !strings.Contains(md, "| RPS | 5.00 | 5.00 |") {
		t.Error("tie row should render both values plainly")
	}
}

func TestMarkdownBenchReport_PKQueueTime(t *testing.T) {
	providers := []ProviderConfig{{Name: "A", URL: "http://a", Model: "m"}, {Name: "B", URL: "http://b", Model: "m"}}
	cfg := BenchConfig{Mode: ModeCompletion, Concurrency: 1, TotalRequests: 1, LoadModel: LoadModelOpenLoop, RequestRate: 10, MaxInFlight: 2}
	a := &CompletionReport{TotalRequests: 1, SuccessCount: 1, Valid: true, QueueTimeAvg: 3, QueueTimeP99: 9}
	b := &CompletionReport{TotalRequests: 1, SuccessCount: 1, Valid: true}
	md := MarkdownBenchReport(ModeCompletion, "pk", providers, cfg, nil, []*CompletionReport{a, b}, time.Now())
	if !strings.Contains(md, "| Queue Time Avg | 3.00 ms | **0.00 ms** |") {
		t.Error("queue time rows should appear when either provider has data")
	}
	if !strings.Contains(md, "| Queue Time P99 | 9.00 ms | **0.00 ms** |") {
		t.Error("queue time winner should be bolded (lower wins)")
	}
}

func TestMarkdownBenchReport_PKBothInvalid(t *testing.T) {
	providers := []ProviderConfig{{Name: "A", URL: "http://a", Model: "m"}, {Name: "B", URL: "http://b", Model: "m"}}
	cfg := BenchConfig{Mode: ModeCompletion, Concurrency: 1, TotalRequests: 1}
	a := &CompletionReport{TotalRequests: 1, ErrorCount: 1, Valid: false}
	b := &CompletionReport{TotalRequests: 1, ErrorCount: 1, Valid: false}
	md := MarkdownBenchReport(ModeCompletion, "pk", providers, cfg, nil, []*CompletionReport{a, b}, time.Now())
	if !strings.Contains(md, "_A: all requests failed_") || !strings.Contains(md, "_B: all requests failed_") {
		t.Error("both failure notes should appear")
	}
}

func TestMarkdownCacheHitReport(t *testing.T) {
	provider := ProviderConfig{Name: "Provider", URL: "http://x/v1/chat/completions", APIKey: "sk-abcdefgh", Model: "qwen3", CustomParams: `{"prompt_cache_key":"benchmark"}`}
	cfg := CacheHitConfig{TestCount: 3, MaxOutputTokens: 1, SystemPrompt: "sys"}
	r := &CacheHitReport{
		TotalRequests: 3, SuccessCount: 2, ErrorCount: 1,
		WallTime: 8 * time.Second, Valid: true,
		TotalPromptTokens: 12000, TotalCachedTokens: 8000,
		CacheHitRate: 66.67, AvgRequestHitRate: 66.67, AvgLatencyMs: 250,
		ErrorCategories: map[string]int{"timeout": 1},
		Results: []CacheHitResult{
			{Index: 1, Latency: 200 * time.Millisecond, PromptTokens: 4000, CachedTokens: 4000, HasCachedTokens: true},
			{Index: 2, Latency: 300 * time.Millisecond, PromptTokens: 4000, HasCachedTokens: false},
			{Index: 3, Err: fmt.Errorf("connection refused")},
		},
	}
	now := time.Date(2026, 7, 29, 15, 30, 45, 0, time.Local)
	md := MarkdownCacheHitReport(provider, cfg, "repeat this prompt", r, now)

	for _, want := range []string{
		"# Prompt Cache Hit Report",
		"## Test Parameters",
		"| API Key | ***efgh |",
		"| Custom Params | {\"prompt_cache_key\":\"benchmark\"} |",
		"| Test Count | 3 |",
		"| Max Output Tokens | 1 |",
		"| Interval | 3s |",
		"| System Prompt | sys |",
		"| User Prompt | repeat this prompt |",
		"## Results",
		"| Successful | 2 (66.7%) |",
		"| Wall Time | 8.00 s |",
		"| Error Categories | timeout: 1 |",
		"| Prompt Tokens | 12000 |",
		"| Cached Tokens | 8000 |",
		"| Overall Hit Rate | 66.67% |",
		"| Avg Latency | 250.00 ms |",
		"## Per-Request Results",
		"| # | Latency | Prompt | Cached | Hit Rate |",
		"| 1 | 200.00 ms | 4000 | 4000 | 100.00% |",
		"| 2 | 300.00 ms | 4000 | N/A | N/A |",
		"| 3 | error: connection refused | | | |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("cache hit report missing %q\n--- report ---\n%s", want, md)
		}
	}
}

func TestMarkdownCacheHitReportAllFailed(t *testing.T) {
	provider := ProviderConfig{Name: "Provider", URL: "http://x", Model: "m"}
	cfg := CacheHitConfig{TestCount: 2}
	r := &CacheHitReport{
		TotalRequests: 2, ErrorCount: 2, Valid: false,
		Results: []CacheHitResult{{Index: 1, Err: fmt.Errorf("boom")}},
	}
	md := MarkdownCacheHitReport(provider, cfg, "p", r, time.Now())
	if !strings.Contains(md, "_All requests failed — no cache metrics available._") {
		t.Error("expected all-failed note")
	}
	if strings.Contains(md, "| Overall Hit Rate |") {
		t.Error("metrics table must be omitted when all requests failed")
	}
	if !strings.Contains(md, "| 1 | error: boom | | | |") {
		t.Error("per-request table must still be present")
	}
}

func TestMarkdownCacheHitReportMissingAndZeroPrompt(t *testing.T) {
	provider := ProviderConfig{Name: "Provider", URL: "http://x", Model: "m"}
	cfg := CacheHitConfig{TestCount: 2, Interval: 5 * time.Second}
	r := &CacheHitReport{
		TotalRequests: 1, SuccessCount: 1, Valid: true,
		MissingUsageCount: 1, MissingCachedCount: 1,
		Results: []CacheHitResult{
			{Index: 1, Latency: 100 * time.Millisecond, PromptTokens: 0, CachedTokens: 0, HasCachedTokens: true},
		},
	}
	md := MarkdownCacheHitReport(provider, cfg, "p", r, time.Now())
	for _, want := range []string{
		"| Missing Usage | 1 |",
		"| Missing Cached Field | 1 |",
		"| Interval | 5s |",
		"| 1 | 100.00 ms | 0 | 0 | N/A |", // PromptTokens==0 → hit rate N/A (division guard)
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q\n--- report ---\n%s", want, md)
		}
	}
}

func TestMarkdownBenchReport_PKFailureNotesEscaped(t *testing.T) {
	providers := []ProviderConfig{
		{Name: "A|B\nev1l", URL: "http://a", Model: "m"},
		{Name: "B", URL: "http://b", Model: "m"},
	}
	cfg := BenchConfig{Mode: ModeCompletion, Concurrency: 1, TotalRequests: 1}
	a := &CompletionReport{TotalRequests: 1, ErrorCount: 1, Valid: false}
	b := &CompletionReport{TotalRequests: 1, SuccessCount: 1, Valid: true}
	md := MarkdownBenchReport(ModeCompletion, "pk", providers, cfg, nil, []*CompletionReport{a, b}, time.Now())
	if !strings.Contains(md, "_A\\|B ev1l: all requests failed_\n") {
		t.Errorf("provider name in failure note must be escaped and single-line\n--- report ---\n%s", md)
	}
}

func TestWriteMarkdownReport(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 7, 29, 15, 30, 45, 0, time.Local)
	name, err := WriteMarkdownReport(dir, "bench_report", "# hello\n", now)
	if err != nil {
		t.Fatalf("WriteMarkdownReport: %v", err)
	}
	if name != "bench_report_20260729_153045.md" {
		t.Errorf("unexpected file name %q", name)
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "# hello\n" {
		t.Errorf("file content = %q", string(data))
	}
}

func TestWriteMarkdownReportBadDir(t *testing.T) {
	name, err := WriteMarkdownReport(filepath.Join(t.TempDir(), "nonexistent", "sub"), "bench_report", "x", time.Now())
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
	if name != "" {
		t.Errorf("expected empty name on error, got %q", name)
	}
}
