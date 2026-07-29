# Auto Markdown Report Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically write a markdown report (test parameters + result tables + log file names) to the current directory every time a benchmark or Prompt Cache Hit run completes.

**Architecture:** Markdown generation lives in `internal/bench/report_markdown.go` as pure data-to-string functions (unit-testable without the TUI). The TUI (`app.go`) calls them when a run fully completes and shows a one-line status (`Report saved: <name>` / warning) on the results screens. The existing manual `ctrl+e` plain-text export is unchanged.

**Tech Stack:** Go, bubbletea TUI, standard library only (`fmt`, `os`, `path/filepath`, `sort`, `strconv`, `strings`, `time`).

**Spec:** `docs/superpowers/specs/2026-07-29-markdown-report-design.md`

---

## File Structure

- **Create** `internal/bench/report_markdown.go` — markdown generation (`MarkdownBenchReport`, `MarkdownCacheHitReport`) + file writer (`WriteMarkdownReport`) + private helpers. All exported functions are pure except `WriteMarkdownReport`.
- **Create** `internal/bench/report_markdown_test.go` — unit tests for all of the above.
- **Modify** `internal/tui/app.go` — write the report on `BenchDoneMsg` (all providers done) and `CacheHitDoneMsg`; store cache-hit config/prompt in `Model`; add `markdownReportDir` package var (`.` in production, tests override).
- **Modify** `internal/tui/results_screen.go` — `reportFile`/`reportErr` fields on `resultsModel` + status line in `view()` + shared `renderReportStatus` helper.
- **Modify** `internal/tui/cache_hit_results.go` — same two fields + status line (both render paths).
- **Modify** `internal/tui/results_screen_test.go` — status-line tests.
- **Create** `internal/tui/report_export_test.go` — end-to-end "message writes file" tests.
- **Modify** `AGENTS.md`, `README.md`, `docs/requirements.md` — documentation.

---

### Task 1: Markdown helpers + Embedding single-provider report

**Files:**
- Create: `internal/bench/report_markdown.go`
- Test: `internal/bench/report_markdown_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/bench/report_markdown_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run 'TestMarkdownBenchReport_EmbeddingSingle|TestMdRedactAPIKey|TestMdEscape' -v`
Expected: FAIL — `undefined: MarkdownBenchReport`

- [ ] **Step 3: Write minimal implementation**

Create `internal/bench/report_markdown.go`:

```go
package bench

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MarkdownBenchReport renders a benchmark report (embedding or completion-like,
// single or PK) as a markdown document. Exactly one of embReports / compReports
// is non-empty.
func MarkdownBenchReport(apiMode, testMode string, providers []ProviderConfig, cfg BenchConfig, embReports []*EmbeddingReport, compReports []*CompletionReport, now time.Time) string {
	var sb strings.Builder
	title := "# Benchmark Report — " + mdModeDisplayName(apiMode)
	if testMode == "pk" {
		title += " (PK)"
	}
	sb.WriteString(title + "\n\n")
	sb.WriteString("Generated: " + now.Format("2006-01-02 15:04:05") + "\n\n")

	sb.WriteString("## Test Parameters\n\n")
	mdWriteBenchParams(&sb, apiMode, testMode, providers, cfg)

	sb.WriteString("## Results\n\n")
	if apiMode == ModeEmbedding {
		if testMode == "pk" {
			mdWriteEmbeddingPK(&sb, providers, embReports)
		} else {
			mdWriteEmbeddingSingle(&sb, firstEmbeddingReport(embReports))
		}
	} else {
		if testMode == "pk" {
			mdWriteCompletionPK(&sb, providers, compReports)
		} else {
			mdWriteCompletionSingle(&sb, firstCompletionReport(compReports))
		}
	}
	return sb.String()
}

func firstEmbeddingReport(reports []*EmbeddingReport) *EmbeddingReport {
	if len(reports) == 0 {
		return nil
	}
	return reports[0]
}

func firstCompletionReport(reports []*CompletionReport) *CompletionReport {
	if len(reports) == 0 {
		return nil
	}
	return reports[0]
}

func mdModeDisplayName(apiMode string) string {
	switch apiMode {
	case ModeEmbedding:
		return "Embedding"
	case ModeAnthropicMessages:
		return "Anthropic Messages"
	default:
		return "Chat Completion"
	}
}

// --- parameters ---

func mdWriteBenchParams(sb *strings.Builder, apiMode, testMode string, providers []ProviderConfig, cfg BenchConfig) {
	if testMode == "pk" && len(providers) >= 2 {
		rows := [][3]string{
			{"URL", providers[0].URL, providers[1].URL},
			{"Model", providers[0].Model, providers[1].Model},
			{"API Key", mdRedactAPIKey(providers[0].APIKey), mdRedactAPIKey(providers[1].APIKey)},
		}
		if providers[0].CustomParams != "" || providers[1].CustomParams != "" {
			rows = append(rows, [3]string{"Custom Params", providers[0].CustomParams, providers[1].CustomParams})
		}
		mdTable3(sb, "Parameter", providers[0].Name, providers[1].Name, rows)
		mdTable(sb, "Parameter", "Value", mdSharedBenchParams(apiMode, testMode, cfg))
		return
	}

	var rows [][]string
	if len(providers) >= 1 {
		p := providers[0]
		rows = append(rows,
			[]string{"Provider", p.Name},
			[]string{"URL", p.URL},
			[]string{"Model", p.Model},
			[]string{"API Key", mdRedactAPIKey(p.APIKey)},
		)
		if p.CustomParams != "" {
			rows = append(rows, []string{"Custom Params", p.CustomParams})
		}
	}
	rows = append(rows, mdSharedBenchParams(apiMode, testMode, cfg)...)
	mdTable(sb, "Parameter", "Value", rows)
}

func mdSharedBenchParams(apiMode, testMode string, cfg BenchConfig) [][]string {
	isCompletionLike := apiMode == ModeCompletion || apiMode == ModeAnthropicMessages
	rows := [][]string{
		{"API Mode", apiMode},
		{"Test Mode", testMode},
	}
	if cfg.LoadModel == LoadModelOpenLoop {
		rows = append(rows,
			[]string{"Load Model", cfg.LoadModel},
			[]string{"Request Rate", fmt.Sprintf("%d req/s", cfg.RequestRate)},
			[]string{"Max In-Flight", fmt.Sprintf("%d", cfg.MaxInFlight)},
		)
	} else {
		rows = append(rows, []string{"Concurrency", fmt.Sprintf("%d", cfg.Concurrency)})
		if apiMode == ModeCompletion {
			rows = append(rows, []string{"Load Model", LoadModelClosedLoop})
		}
	}
	rows = append(rows,
		[]string{"Total Requests", fmt.Sprintf("%d", cfg.TotalRequests)},
		[]string{"Input Tokens", fmt.Sprintf("%d", cfg.TargetTokens)},
	)
	if isCompletionLike {
		rows = append(rows, []string{"Max Output Tokens", mdMaxOutputTokens(cfg.MaxOutputTokens)})
		rows = append(rows, []string{"TTFT Includes Reasoning", strconv.FormatBool(cfg.TTFTIncludesReasoning)})
		if cfg.SystemPrompt != "" {
			rows = append(rows, []string{"System Prompt", cfg.SystemPrompt})
		}
	}
	if apiMode == ModeCompletion && testMode == "single" {
		rows = append(rows, []string{"Request Logging", strconv.FormatBool(cfg.RequestLogging)})
	}
	return rows
}

func mdMaxOutputTokens(n int) string {
	if n <= 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", n)
}

// --- embedding single results ---

func mdWriteEmbeddingSingle(sb *strings.Builder, r *EmbeddingReport) {
	if r == nil {
		sb.WriteString("_No results available._\n")
		return
	}
	mdTable(sb, "Metric", "Value", mdSummaryRows(r.TotalRequests, r.SuccessCount, r.ErrorCount, r.ErrorCategories))
	if !r.Valid {
		sb.WriteString("_All requests failed — no metrics available._\n")
		return
	}
	mdTable(sb, "Metric", "Value", [][]string{
		{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())},
		{"RPS", mdFmtF(r.RPS, 2)},
		{"Input TPS", mdFmtF(r.InputTPS, 1)},
		{"Input TPM", mdFmtF(r.InputTPM, 0)},
		{"Latency Avg", mdFmtMs(r.LatencyAvg)},
		{"Latency P50", mdFmtMs(r.LatencyP50)},
		{"Latency P90", mdFmtMs(r.LatencyP90)},
		{"Latency P99", mdFmtMs(r.LatencyP99)},
	})
}

func mdSummaryRows(total, success, failed int, categories map[string]int) [][]string {
	successPct := 0.0
	if total > 0 {
		successPct = float64(success) / float64(total) * 100
	}
	rows := [][]string{
		{"Total Requests", fmt.Sprintf("%d", total)},
		{"Successful", fmt.Sprintf("%d (%.1f%%)", success, successPct)},
		{"Failed", fmt.Sprintf("%d", failed)},
	}
	if summary := mdSummarizeErrorCategories(categories); summary != "" {
		rows = append(rows, []string{"Error Categories", summary})
	}
	return rows
}

// --- helpers ---

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

func mdRedactAPIKey(key string) string {
	if key == "" {
		return "(none)"
	}
	if len(key) <= 4 {
		return "***"
	}
	return "***" + key[len(key)-4:]
}

func mdTable(sb *strings.Builder, col1, col2 string, rows [][]string) {
	fmt.Fprintf(sb, "| %s | %s |\n|---|---|\n", col1, col2)
	for _, r := range rows {
		if len(r) < 2 {
			continue
		}
		fmt.Fprintf(sb, "| %s | %s |\n", mdEscape(r[0]), mdEscape(r[1]))
	}
	sb.WriteString("\n")
}

func mdTable3(sb *strings.Builder, col1, col2, col3 string, rows [][3]string) {
	fmt.Fprintf(sb, "| %s | %s | %s |\n|---|---|---|\n", mdEscape(col1), mdEscape(col2), mdEscape(col3))
	for _, r := range rows {
		fmt.Fprintf(sb, "| %s | %s | %s |\n", mdEscape(r[0]), mdEscape(r[1]), mdEscape(r[2]))
	}
	sb.WriteString("\n")
}

func mdFmtF(v float64, decimals int) string {
	return fmt.Sprintf(fmt.Sprintf("%%.%df", decimals), v)
}

func mdFmtMs(v float64) string { return fmt.Sprintf("%.2f ms", v) }

func mdFmtMsPerTok(v float64) string { return fmt.Sprintf("%.2f ms/tok", v) }

func mdSummarizeErrorCategories(categories map[string]int) string {
	if len(categories) == 0 {
		return ""
	}
	type kv struct {
		name  string
		count int
	}
	pairs := make([]kv, 0, len(categories))
	for name, count := range categories {
		if count > 0 {
			pairs = append(pairs, kv{name, count})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].name < pairs[j].name
	})
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s: %d", p.name, p.count))
	}
	return strings.Join(parts, ", ")
}
```

Note: `mdWriteEmbeddingPK` / `mdWriteCompletionPK` / `mdWriteCompletionSingle` are called by `MarkdownBenchReport` but not yet defined — add temporary stubs at the bottom so the package compiles (they get real implementations in Tasks 2–3):

```go
// Temporary stubs — implemented in later tasks.
func mdWriteEmbeddingPK(sb *strings.Builder, providers []ProviderConfig, reports []*EmbeddingReport) {
}
func mdWriteCompletionSingle(sb *strings.Builder, r *CompletionReport) {}
func mdWriteCompletionPK(sb *strings.Builder, providers []ProviderConfig, reports []*CompletionReport) {
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bench/ -run 'TestMarkdownBenchReport_EmbeddingSingle|TestMdRedactAPIKey|TestMdEscape' -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/bench/report_markdown.go internal/bench/report_markdown_test.go
git commit -m "feat: add markdown report generation for embedding single-provider runs"
```

---

### Task 2: Completion single-provider report (API usage, logs, queue time)

**Files:**
- Modify: `internal/bench/report_markdown.go`
- Test: `internal/bench/report_markdown_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/bench/report_markdown_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run 'TestMarkdownBenchReport_Completion|TestMarkdownBenchReport_OpenLoop|TestMarkdownBenchReport_PipeEscaped' -v`
Expected: FAIL — completion content missing (stub renders nothing)

- [ ] **Step 3: Write implementation**

In `internal/bench/report_markdown.go`, replace the `mdWriteCompletionSingle` stub with:

```go
func mdWriteCompletionSingle(sb *strings.Builder, r *CompletionReport) {
	if r == nil {
		sb.WriteString("_No results available._\n")
		return
	}
	mdTable(sb, "Metric", "Value", mdSummaryRows(r.TotalRequests, r.SuccessCount, r.ErrorCount, r.ErrorCategories))
	if !r.Valid {
		sb.WriteString("_All requests failed — no metrics available._\n")
		mdWriteLogSection(sb, r)
		return
	}
	mdTable(sb, "Metric", "Value", [][]string{
		{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())},
		{"RPS", mdFmtF(r.RPS, 2)},
		{"Input TPS", mdFmtF(r.InputTPS, 1)},
		{"Output TPS", mdFmtF(r.OutputTPS, 1)},
		{"Input TPM", mdFmtF(r.InputTPM, 0)},
		{"Output TPM", mdFmtF(r.OutputTPM, 0)},
		{"Avg Output Tokens", mdFmtF(r.AvgOutputTokens, 1)},
	})
	mdTable(sb, "Metric", "Value", mdAPIUsageRows(r))
	mdTable(sb, "Metric", "Value", [][]string{
		{"TTFT Avg", mdFmtMs(r.TTFTAvg)},
		{"TTFT P50", mdFmtMs(r.TTFTp50)},
		{"TTFT P90", mdFmtMs(r.TTFTp90)},
		{"TTFT P99", mdFmtMs(r.TTFTp99)},
	})
	mdTable(sb, "Metric", "Value", [][]string{
		{"TPOT Avg", mdFmtMsPerTok(r.TPOTAvg)},
		{"TPOT P50", mdFmtMsPerTok(r.TPOTp50)},
		{"TPOT P90", mdFmtMsPerTok(r.TPOTp90)},
		{"TPOT P99", mdFmtMsPerTok(r.TPOTp99)},
	})
	e2e := [][]string{
		{"E2E Avg", mdFmtMs(r.E2EAvg)},
		{"E2E P50", mdFmtMs(r.E2Ep50)},
		{"E2E P90", mdFmtMs(r.E2Ep90)},
		{"E2E P99", mdFmtMs(r.E2Ep99)},
	}
	if r.SkippedChunks > 0 {
		e2e = append(e2e, []string{"Skipped SSE Chunks", fmt.Sprintf("%d (warning)", r.SkippedChunks)})
	}
	mdTable(sb, "Metric", "Value", e2e)
	if r.QueueTimeAvg > 0 || r.QueueTimeP50 > 0 || r.QueueTimeP90 > 0 || r.QueueTimeP99 > 0 {
		mdTable(sb, "Metric", "Value", [][]string{
			{"Queue Time Avg", mdFmtMs(r.QueueTimeAvg)},
			{"Queue Time P50", mdFmtMs(r.QueueTimeP50)},
			{"Queue Time P90", mdFmtMs(r.QueueTimeP90)},
			{"Queue Time P99", mdFmtMs(r.QueueTimeP99)},
		})
	}
	mdWriteLogSection(sb, r)
}

func mdAPIUsageRows(r *CompletionReport) [][]string {
	rows := [][]string{
		{"API Prompt Tokens", mdAPIUsageTokens(r, r.APIPromptTokens)},
		{"API Completion Tokens", mdAPIUsageTokens(r, r.APICompletionTokens)},
		{"API Total Tokens", mdAPIUsageTokens(r, r.APITotalTokens)},
		{"API Usage Samples", mdAPIUsageSamples(r)},
	}
	if r.MissingAPIUsageCount > 0 {
		rows = append(rows, []string{"Missing API Usage", fmt.Sprintf("%d", r.MissingAPIUsageCount)})
	}
	return rows
}

func mdAPIUsageTokens(r *CompletionReport, value int) string {
	if r.APIUsageCount == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d", value)
}

func mdAPIUsageSamples(r *CompletionReport) string {
	if r.SuccessCount == 0 {
		return "N/A"
	}
	if r.APIUsageCount == 0 {
		return fmt.Sprintf("N/A / %d", r.SuccessCount)
	}
	return fmt.Sprintf("%d / %d", r.APIUsageCount, r.SuccessCount)
}

func mdWriteLogSection(sb *strings.Builder, r *CompletionReport) {
	if r.LogRequestsFile == "" {
		return
	}
	sb.WriteString("## Logs\n\n")
	fmt.Fprintf(sb, "- Requests: `%s`\n", r.LogRequestsFile)
	fmt.Fprintf(sb, "- Responses: `%s`\n", r.LogResponsesFile)
	if r.LogDroppedCount > 0 {
		fmt.Fprintf(sb, "- ⚠ %d log entries dropped (disk too slow)\n", r.LogDroppedCount)
	}
	if r.LogError != "" {
		fmt.Fprintf(sb, "- ⚠ log write error: %s\n", r.LogError)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bench/ -run 'TestMarkdownBenchReport' -v`
Expected: PASS (all `TestMarkdownBenchReport_*` including Task 1 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/bench/report_markdown.go internal/bench/report_markdown_test.go
git commit -m "feat: markdown report for completion single-provider runs (API usage, logs, queue time)"
```

---

### Task 3: PK reports (embedding + completion)

**Files:**
- Modify: `internal/bench/report_markdown.go`
- Test: `internal/bench/report_markdown_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/bench/report_markdown_test.go`:

```go
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
		"| RPS | **9.80** | 8.10 |",        // higher wins → Alpha bold
		"| Output TPS | 300.0 | **320.0** |", // Beta bold
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run 'TestMarkdownBenchReport_CompletionPK|TestMarkdownBenchReport_EmbeddingPK|TestMarkdownBenchReport_PKInvalidProvider' -v`
Expected: FAIL — PK stubs render nothing

- [ ] **Step 3: Write implementation**

In `internal/bench/report_markdown.go`, replace the two PK stubs with:

```go
type mdPKRow struct {
	metric     string
	valA, valB string
	winner     int // 0 = A wins, 1 = B wins, -1 = tie/na
}

type mdMetricDef struct {
	metric         string
	valA, valB     float64
	format         func(float64) string
	higherIsBetter bool
}

func mdPKMetricRows(defs []mdMetricDef) []mdPKRow {
	rows := make([]mdPKRow, 0, len(defs))
	for _, d := range defs {
		rows = append(rows, mdPKRow{
			metric: d.metric,
			valA:   d.format(d.valA),
			valB:   d.format(d.valB),
			winner: mdWinner(d.valA, d.valB, d.higherIsBetter),
		})
	}
	return rows
}

func mdWinner(a, b float64, higherIsBetter bool) int {
	if a == b {
		return -1
	}
	if higherIsBetter {
		if a > b {
			return 0
		}
		return 1
	}
	if a < b {
		return 0
	}
	return 1
}

func mdWriteEmbeddingPK(sb *strings.Builder, providers []ProviderConfig, reports []*EmbeddingReport) {
	var a, b *EmbeddingReport
	if len(reports) > 0 {
		a = reports[0]
	}
	if len(reports) > 1 {
		b = reports[1]
	}
	nameA, nameB := mdPKNames(providers)
	aValid := a != nil && a.Valid
	bValid := b != nil && b.Valid

	rows := []mdPKRow{
		{metric: "Success / Total", valA: mdEmbeddingSuccessTotal(a), valB: mdEmbeddingSuccessTotal(b), winner: -1},
		{metric: "Error Categories", valA: mdSummarizeErrorCategories(embeddingCategories(a)), valB: mdSummarizeErrorCategories(embeddingCategories(b)), winner: -1},
	}
	if aValid && bValid {
		rows = append(rows, mdPKMetricRows([]mdMetricDef{
			{"RPS", a.RPS, b.RPS, func(v float64) string { return mdFmtF(v, 2) }, true},
			{"Input TPS", a.InputTPS, b.InputTPS, func(v float64) string { return mdFmtF(v, 1) }, true},
			{"Input TPM", a.InputTPM, b.InputTPM, func(v float64) string { return mdFmtF(v, 0) }, true},
			{"Latency Avg", a.LatencyAvg, b.LatencyAvg, mdFmtMs, false},
			{"Latency P50", a.LatencyP50, b.LatencyP50, mdFmtMs, false},
			{"Latency P90", a.LatencyP90, b.LatencyP90, mdFmtMs, false},
			{"Latency P99", a.LatencyP99, b.LatencyP99, mdFmtMs, false},
		})...)
	}
	mdPKTable(sb, nameA, nameB, rows)
	mdPKFailureNotes(sb, nameA, aValid, nameB, bValid)
}

func mdWriteCompletionPK(sb *strings.Builder, providers []ProviderConfig, reports []*CompletionReport) {
	var a, b *CompletionReport
	if len(reports) > 0 {
		a = reports[0]
	}
	if len(reports) > 1 {
		b = reports[1]
	}
	nameA, nameB := mdPKNames(providers)
	aValid := a != nil && a.Valid
	bValid := b != nil && b.Valid

	rows := []mdPKRow{
		{metric: "Success / Total", valA: mdCompletionSuccessTotal(a), valB: mdCompletionSuccessTotal(b), winner: -1},
		{metric: "Error Categories", valA: mdSummarizeErrorCategories(completionCategories(a)), valB: mdSummarizeErrorCategories(completionCategories(b)), winner: -1},
		{metric: "API Prompt Tokens", valA: mdPKAPIUsageTokens(a, func(r *CompletionReport) int { return r.APIPromptTokens }), valB: mdPKAPIUsageTokens(b, func(r *CompletionReport) int { return r.APIPromptTokens }), winner: -1},
		{metric: "API Completion Tokens", valA: mdPKAPIUsageTokens(a, func(r *CompletionReport) int { return r.APICompletionTokens }), valB: mdPKAPIUsageTokens(b, func(r *CompletionReport) int { return r.APICompletionTokens }), winner: -1},
		{metric: "API Total Tokens", valA: mdPKAPIUsageTokens(a, func(r *CompletionReport) int { return r.APITotalTokens }), valB: mdPKAPIUsageTokens(b, func(r *CompletionReport) int { return r.APITotalTokens }), winner: -1},
		{metric: "API Usage Samples", valA: mdPKAPIUsageSamples(a), valB: mdPKAPIUsageSamples(b), winner: -1},
	}
	if aValid && bValid {
		defs := []mdMetricDef{
			{"RPS", a.RPS, b.RPS, func(v float64) string { return mdFmtF(v, 2) }, true},
			{"Input TPS", a.InputTPS, b.InputTPS, func(v float64) string { return mdFmtF(v, 1) }, true},
			{"Output TPS", a.OutputTPS, b.OutputTPS, func(v float64) string { return mdFmtF(v, 1) }, true},
			{"Input TPM", a.InputTPM, b.InputTPM, func(v float64) string { return mdFmtF(v, 0) }, true},
			{"Output TPM", a.OutputTPM, b.OutputTPM, func(v float64) string { return mdFmtF(v, 0) }, true},
			{"TTFT Avg", a.TTFTAvg, b.TTFTAvg, mdFmtMs, false},
			{"TTFT P50", a.TTFTp50, b.TTFTp50, mdFmtMs, false},
			{"TTFT P90", a.TTFTp90, b.TTFTp90, mdFmtMs, false},
			{"TTFT P99", a.TTFTp99, b.TTFTp99, mdFmtMs, false},
			{"TPOT Avg", a.TPOTAvg, b.TPOTAvg, mdFmtMsPerTok, false},
			{"TPOT P50", a.TPOTp50, b.TPOTp50, mdFmtMsPerTok, false},
			{"TPOT P90", a.TPOTp90, b.TPOTp90, mdFmtMsPerTok, false},
			{"TPOT P99", a.TPOTp99, b.TPOTp99, mdFmtMsPerTok, false},
			{"E2E Avg", a.E2EAvg, b.E2EAvg, mdFmtMs, false},
			{"E2E P50", a.E2Ep50, b.E2Ep50, mdFmtMs, false},
			{"E2E P90", a.E2Ep90, b.E2Ep90, mdFmtMs, false},
			{"E2E P99", a.E2Ep99, b.E2Ep99, mdFmtMs, false},
		}
		if a.QueueTimeAvg > 0 || b.QueueTimeAvg > 0 {
			defs = append(defs,
				mdMetricDef{"Queue Time Avg", a.QueueTimeAvg, b.QueueTimeAvg, mdFmtMs, false},
				mdMetricDef{"Queue Time P50", a.QueueTimeP50, b.QueueTimeP50, mdFmtMs, false},
				mdMetricDef{"Queue Time P90", a.QueueTimeP90, b.QueueTimeP90, mdFmtMs, false},
				mdMetricDef{"Queue Time P99", a.QueueTimeP99, b.QueueTimeP99, mdFmtMs, false},
			)
		}
		rows = append(rows, mdPKMetricRows(defs)...)
	}
	mdPKTable(sb, nameA, nameB, rows)
	mdPKFailureNotes(sb, nameA, aValid, nameB, bValid)
}

func mdPKNames(providers []ProviderConfig) (string, string) {
	nameA, nameB := "Provider A", "Provider B"
	if len(providers) > 0 && providers[0].Name != "" {
		nameA = providers[0].Name
	}
	if len(providers) > 1 && providers[1].Name != "" {
		nameB = providers[1].Name
	}
	return nameA, nameB
}

func mdEmbeddingSuccessTotal(r *EmbeddingReport) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d / %d", r.SuccessCount, r.TotalRequests)
}

func mdCompletionSuccessTotal(r *CompletionReport) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d / %d", r.SuccessCount, r.TotalRequests)
}

func embeddingCategories(r *EmbeddingReport) map[string]int {
	if r == nil {
		return nil
	}
	return r.ErrorCategories
}

func completionCategories(r *CompletionReport) map[string]int {
	if r == nil {
		return nil
	}
	return r.ErrorCategories
}

func mdPKAPIUsageTokens(r *CompletionReport, value func(*CompletionReport) int) string {
	if r == nil || r.APIUsageCount == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d", value(r))
}

func mdPKAPIUsageSamples(r *CompletionReport) string {
	if r == nil || r.SuccessCount == 0 {
		return "N/A"
	}
	if r.APIUsageCount == 0 {
		return fmt.Sprintf("N/A / %d", r.SuccessCount)
	}
	return fmt.Sprintf("%d / %d", r.APIUsageCount, r.SuccessCount)
}

func mdPKTable(sb *strings.Builder, nameA, nameB string, rows []mdPKRow) {
	fmt.Fprintf(sb, "| Metric | %s | %s |\n", mdEscape(nameA), mdEscape(nameB))
	sb.WriteString("|---|---|---|\n")
	for _, r := range rows {
		va, vb := mdEscape(r.valA), mdEscape(r.valB)
		if r.winner == 0 {
			va = "**" + va + "**"
		}
		if r.winner == 1 {
			vb = "**" + vb + "**"
		}
		fmt.Fprintf(sb, "| %s | %s | %s |\n", mdEscape(r.metric), va, vb)
	}
	sb.WriteString("\n")
}

func mdPKFailureNotes(sb *strings.Builder, nameA string, aValid bool, nameB string, bValid bool) {
	if !aValid {
		fmt.Fprintf(sb, "_%s: all requests failed_\n", nameA)
	}
	if !bValid {
		fmt.Fprintf(sb, "_%s: all requests failed_\n", nameB)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bench/ -v -run 'TestMarkdownBenchReport'`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add internal/bench/report_markdown.go internal/bench/report_markdown_test.go
git commit -m "feat: markdown report for PK mode with winner highlighting"
```

---

### Task 4: Cache Hit markdown report

**Files:**
- Modify: `internal/bench/report_markdown.go`
- Test: `internal/bench/report_markdown_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/bench/report_markdown_test.go`:

```go
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
```

Add `"fmt"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run 'TestMarkdownCacheHitReport' -v`
Expected: FAIL — `undefined: MarkdownCacheHitReport`

- [ ] **Step 3: Write implementation**

Append to `internal/bench/report_markdown.go`:

```go
// MarkdownCacheHitReport renders a Prompt Cache Hit test report as markdown.
func MarkdownCacheHitReport(provider ProviderConfig, cfg CacheHitConfig, userPrompt string, r *CacheHitReport, now time.Time) string {
	var sb strings.Builder
	sb.WriteString("# Prompt Cache Hit Report\n\n")
	sb.WriteString("Generated: " + now.Format("2006-01-02 15:04:05") + "\n\n")

	sb.WriteString("## Test Parameters\n\n")
	interval := cfg.Interval
	if interval <= 0 {
		interval = 3 * time.Second // same default as RunCacheHitTest
	}
	params := [][]string{
		{"Provider", provider.Name},
		{"URL", provider.URL},
		{"Model", provider.Model},
		{"API Key", mdRedactAPIKey(provider.APIKey)},
	}
	if provider.CustomParams != "" {
		params = append(params, []string{"Custom Params", provider.CustomParams})
	}
	params = append(params,
		[]string{"Test Count", fmt.Sprintf("%d", cfg.TestCount)},
		[]string{"Max Output Tokens", mdMaxOutputTokens(cfg.MaxOutputTokens)},
		[]string{"Interval", interval.String()},
	)
	if cfg.SystemPrompt != "" {
		params = append(params, []string{"System Prompt", cfg.SystemPrompt})
	}
	params = append(params, []string{"User Prompt", userPrompt})
	mdTable(&sb, "Parameter", "Value", params)

	sb.WriteString("## Results\n\n")
	if r == nil {
		sb.WriteString("_No results available._\n")
		return sb.String()
	}
	summary := mdSummaryRows(r.TotalRequests, r.SuccessCount, r.ErrorCount, r.ErrorCategories)
	summary = append(summary, []string{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())})
	mdTable(&sb, "Metric", "Value", summary)

	if !r.Valid {
		sb.WriteString("_All requests failed — no cache metrics available._\n\n")
	} else {
		metrics := [][]string{
			{"Prompt Tokens", fmt.Sprintf("%d", r.TotalPromptTokens)},
			{"Cached Tokens", fmt.Sprintf("%d", r.TotalCachedTokens)},
			{"Overall Hit Rate", fmt.Sprintf("%.2f%%", r.CacheHitRate)},
			{"Avg Request Hit Rate", fmt.Sprintf("%.2f%%", r.AvgRequestHitRate)},
			{"Avg Latency", mdFmtMs(r.AvgLatencyMs)},
		}
		if r.MissingUsageCount > 0 {
			metrics = append(metrics, []string{"Missing Usage", fmt.Sprintf("%d", r.MissingUsageCount)})
		}
		if r.MissingCachedCount > 0 {
			metrics = append(metrics, []string{"Missing Cached Field", fmt.Sprintf("%d", r.MissingCachedCount)})
		}
		mdTable(&sb, "Metric", "Value", metrics)
	}

	sb.WriteString("## Per-Request Results\n\n")
	sb.WriteString("| # | Latency | Prompt | Cached | Hit Rate |\n")
	sb.WriteString("|---|---|---|---|---|\n")
	for _, res := range r.Results {
		if res.Err != nil {
			fmt.Fprintf(&sb, "| %d | error: %s | | | |\n", res.Index, mdEscape(res.Err.Error()))
			continue
		}
		cached := "N/A"
		hitRate := "N/A"
		if res.HasCachedTokens {
			cached = fmt.Sprintf("%d", res.CachedTokens)
			if res.PromptTokens > 0 {
				hitRate = fmt.Sprintf("%.2f%%", float64(res.CachedTokens)/float64(res.PromptTokens)*100)
			}
		}
		fmt.Fprintf(&sb, "| %d | %s | %d | %s | %s |\n",
			res.Index, mdFmtMs(float64(res.Latency.Milliseconds())), res.PromptTokens, cached, hitRate)
	}
	return sb.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bench/ -run 'TestMarkdownCacheHitReport' -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/bench/report_markdown.go internal/bench/report_markdown_test.go
git commit -m "feat: markdown report for prompt cache hit tests"
```

---

### Task 5: WriteMarkdownReport file writer

**Files:**
- Modify: `internal/bench/report_markdown.go`
- Test: `internal/bench/report_markdown_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/bench/report_markdown_test.go`:

```go
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
	_, err := WriteMarkdownReport(filepath.Join(t.TempDir(), "nonexistent", "sub"), "bench_report", "x", time.Now())
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}
```

Add `"os"` and `"path/filepath"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run 'TestWriteMarkdownReport' -v`
Expected: FAIL — `undefined: WriteMarkdownReport`

- [ ] **Step 3: Write implementation**

Append to `internal/bench/report_markdown.go` (add `"os"` and `"path/filepath"` to its imports):

```go
// WriteMarkdownReport writes content to dir/<prefix>_YYYYMMDD_HHMMSS.md
// (timestamp from now, local time) and returns the file name (not the full path).
func WriteMarkdownReport(dir, prefix, content string, now time.Time) (string, error) {
	name := fmt.Sprintf("%s_%s.md", prefix, now.Format("20060102_150405"))
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		return "", err
	}
	return name, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/bench/ -v`
Expected: PASS (entire bench package)

- [ ] **Step 5: Commit**

```bash
git add internal/bench/report_markdown.go internal/bench/report_markdown_test.go
git commit -m "feat: add WriteMarkdownReport timestamped file writer"
```

---

### Task 6: TUI wiring — benchmark results screen

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/results_screen.go`
- Test: `internal/tui/report_export_test.go` (create)
- Test: `internal/tui/results_screen_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/tui/report_export_test.go`:

```go
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
```

Also append to `internal/tui/results_screen_test.go`:

```go
func TestResultsViewShowsReportStatus(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"p"})
	m.addCompletionResult(0, fullCompletionReport())
	m.setSize(100, 24)
	m.reportFile = "bench_report_20260729_153045.md"

	out := m.view(100, 24)
	if !strings.Contains(out, "Report saved: bench_report_20260729_153045.md") {
		t.Error("results view should show the saved report file name")
	}
	if n := strings.Count(out, "\n") + 1; n > 24 {
		t.Errorf("results view with status line rendered %d lines, exceeds terminal height 24", n)
	}
}

func TestResultsViewShowsReportError(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"p"})
	m.addCompletionResult(0, fullCompletionReport())
	m.setSize(100, 24)
	m.reportErr = "permission denied"

	out := m.view(100, 24)
	if !strings.Contains(out, "failed to write report: permission denied") {
		t.Error("results view should show the report write error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestBenchDoneMsg|TestResultsViewShowsReport' -v`
Expected: FAIL — `undefined: markdownReportDir`, `m.reportFile undefined`

- [ ] **Step 3: Write implementation**

In `internal/tui/results_screen.go`, add the fields to `resultsModel` (after `hasErrors`):

```go
	hasErrors         bool
	reportFile        string // auto-written markdown report file name ("" = not written)
	reportErr         string // markdown report write error, if any
```

Add a package-level helper at the end of the file:

```go
// renderReportStatus renders the auto markdown report status line shown above
// the hints on result screens.
func renderReportStatus(sb *strings.Builder, reportFile, reportErr string) {
	if reportFile != "" {
		sb.WriteString(dimStyle.Render("  Report saved: " + reportFile))
		sb.WriteString("\n")
	} else if reportErr != "" {
		sb.WriteString(errorStyle.Render("  ⚠ failed to write report: " + reportErr))
		sb.WriteString("\n")
	}
}
```

In `resultsModel.view()`, render the status between the viewport and the hints:

```go
	sb.WriteString(m.vp.View())
	sb.WriteString("\n")
	renderReportStatus(&sb, m.reportFile, m.reportErr)
	sb.WriteString(helpStyle.Render(m.hints()))
```

In `internal/tui/app.go`:

1. Add `"time"` to the imports.
2. Add the package-level var near `prog`:

```go
// markdownReportDir is where auto markdown reports are written.
// "." in production; tests override it.
var markdownReportDir = "."
```

3. In the `BenchDoneMsg` case, replace the `if m.doneProviders >= len(m.providers)` block:

```go
		if m.doneProviders >= len(m.providers) {
			// All done — write the auto markdown report, then go to results
			now := time.Now()
			content := bench.MarkdownBenchReport(m.apiMode, m.testMode, m.providers, m.cfg, m.results.embeddingReports, m.results.completionReports, now)
			name, err := bench.WriteMarkdownReport(markdownReportDir, "bench_report", content, now)
			if err != nil {
				m.results.reportErr = err.Error()
			} else {
				m.results.reportFile = name
			}
			m.screen = ScreenResults
		}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -v`
Expected: PASS (entire tui package, including pre-existing tests)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/results_screen.go internal/tui/report_export_test.go internal/tui/results_screen_test.go
git commit -m "feat: auto-write markdown report when benchmark completes"
```

---

### Task 7: TUI wiring — cache hit results screen

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/cache_hit_results.go`
- Test: `internal/tui/report_export_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/report_export_test.go`:

```go
func TestCacheHitDoneMsgWritesMarkdownReport(t *testing.T) {
	dir := t.TempDir()
	old := markdownReportDir
	markdownReportDir = dir
	defer func() { markdownReportDir = old }()

	m := NewModel(nil)
	m.providers = []bench.ProviderConfig{{Name: "Provider", URL: "http://x", Model: "m"}}
	m.cacheHitCfg = bench.CacheHitConfig{TestCount: 2}
	m.cacheHitPrompt = "repeat me"

	updated, _ := m.Update(CacheHitDoneMsg{
		Report: &bench.CacheHitReport{TotalRequests: 2, SuccessCount: 2, Valid: true},
	})
	um := updated.(Model)

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.HasPrefix(files[0].Name(), "cache_hit_report_") {
		t.Fatalf("expected one cache_hit_report_*.md file, got %v", files)
	}
	data, _ := os.ReadFile(filepath.Join(dir, files[0].Name()))
	content := string(data)
	for _, want := range []string{"# Prompt Cache Hit Report", "| User Prompt | repeat me |"} {
		if !strings.Contains(content, want) {
			t.Errorf("cache hit report missing %q", want)
		}
	}
	if um.cacheHitResults.reportFile != files[0].Name() {
		t.Errorf("cacheHitResults.reportFile = %q", um.cacheHitResults.reportFile)
	}
	if um.screen != ScreenCacheHitResults {
		t.Error("should still transition to cache hit results screen")
	}
}
```

And a view test — append to `internal/tui/results_screen_test.go`:

```go
func TestCacheHitResultsViewShowsReportStatus(t *testing.T) {
	m := newCacheHitResultsModel()
	m.setReport(&bench.CacheHitReport{TotalRequests: 1, SuccessCount: 1, Valid: true})
	m.reportFile = "cache_hit_report_20260729_153045.md"
	out := m.view(100, 24)
	if !strings.Contains(out, "Report saved: cache_hit_report_20260729_153045.md") {
		t.Error("cache hit results view should show the saved report file name")
	}
}

func TestCacheHitResultsViewShowsReportStatusWhenAllFailed(t *testing.T) {
	m := newCacheHitResultsModel()
	m.setReport(&bench.CacheHitReport{TotalRequests: 1, ErrorCount: 1, Valid: false})
	m.reportFile = "cache_hit_report_20260729_153045.md"
	out := m.view(100, 24)
	if !strings.Contains(out, "Report saved: cache_hit_report_20260729_153045.md") {
		t.Error("status line must also appear on the all-failed view path")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestCacheHit' -v`
Expected: FAIL — `m.cacheHitCfg undefined`, `m.reportFile undefined`

- [ ] **Step 3: Write implementation**

In `internal/tui/cache_hit_results.go`, add fields to `cacheHitResultsModel`:

```go
type cacheHitResultsModel struct {
	report     *bench.CacheHitReport
	width      int
	reportFile string // auto-written markdown report file name ("" = not written)
	reportErr  string // markdown report write error, if any
}
```

In its `view()`: in the `!r.Valid` early-return path, insert `renderReportStatus(&sb, m.reportFile, m.reportErr)` immediately before the hints line; and at the end of the normal path, insert the same call immediately before the final hints write. The normal path ends with:

```go
	sb.WriteString("\n")
	renderReportStatus(&sb, m.reportFile, m.reportErr)
	hints := "r rerun  •  ctrl+e export  •  esc config  •  ctrl+c quit"
	if m.hasErrors() {
		hints = "r rerun  •  e errors  •  ctrl+e export  •  esc config  •  ctrl+c quit"
	}
	sb.WriteString(helpStyle.Render(hints))
```

In `internal/tui/app.go`:

1. Add fields to `Model` (next to the cache hit flow fields):

```go
	// Prompt cache hit test flow
	cacheHitConfig  cacheHitConfigModel
	cacheHitResults cacheHitResultsModel
	cacheHitCfg     bench.CacheHitConfig // stored for the markdown report
	cacheHitPrompt  string               // stored for the markdown report
```

2. In `startCacheHitRunning`, store them (right after `m.providers = ...`):

```go
	m.cacheHitCfg = cfg
	m.cacheHitPrompt = userPrompt
```

3. In the `CacheHitDoneMsg` case:

```go
	case CacheHitDoneMsg:
		m.cacheHitResults.setReport(msg.Report)
		m.errorViewport.setContent(m.cacheHitResults.errorCategories(), m.cacheHitResults.errorDetails())
		now := time.Now()
		if len(m.providers) > 0 {
			content := bench.MarkdownCacheHitReport(m.providers[0], m.cacheHitCfg, m.cacheHitPrompt, msg.Report, now)
			name, err := bench.WriteMarkdownReport(markdownReportDir, "cache_hit_report", content, now)
			if err != nil {
				m.cacheHitResults.reportErr = err.Error()
			} else {
				m.cacheHitResults.reportFile = name
			}
		}
		m.screen = ScreenCacheHitResults
		return m, nil
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/cache_hit_results.go internal/tui/report_export_test.go internal/tui/results_screen_test.go
git commit -m "feat: auto-write markdown report when cache hit test completes"
```

---

### Task 8: Documentation + full verification

**Files:**
- Modify: `AGENTS.md`
- Modify: `README.md`
- Modify: `docs/requirements.md`

- [ ] **Step 1: Update AGENTS.md**

In the package layout block, add after `openloop.go` line:

```
    report_markdown.go          -- Markdown report generation (MarkdownBenchReport, MarkdownCacheHitReport) + WriteMarkdownReport
```

Add a bullet under "Key Design Decisions" (after the "Request logging" bullet):

```markdown
- **Auto markdown report**: When a benchmark (all providers done) or cache-hit run completes, `app.go` writes `bench_report_<YYYYMMDD_HHMMSS>.md` / `cache_hit_report_<YYYYMMDD_HHMMSS>.md` to the working directory via `bench.MarkdownBenchReport` / `MarkdownCacheHitReport` / `WriteMarkdownReport` (PK mode = one combined file). The file records test parameters (API key redacted `***last4`) and the same metrics as the results screen in markdown tables (PK winner **bold**, invalid provider `N/A`), plus log file names when Request Logging was on. Results screens show `Report saved: <name>` or a write-failure warning. Write errors never affect result display. The manual `ctrl+e` plain-text export is unchanged. Tests redirect output via the unexported `markdownReportDir` package var in `tui`.
```

- [ ] **Step 2: Update README.md**

Add a new section after the `## 请求日志（Request Logging）` section (before `## 思考（Reasoning / Thinking）模型注意事项`):

```markdown
## 自动 Markdown 报告

压测（Embedding / Chat Completion / Anthropic Messages，单 Provider 与 PK）和 Prompt Cache Hit 测试完成后，会在当前目录自动生成一份 markdown 报告，无需手动操作：

- 压测报告：`bench_report_YYYYMMDD_HHMMSS.md`（PK 模式为单个合并文件，含两家对比）
- Cache Hit 报告：`cache_hit_report_YYYYMMDD_HHMMSS.md`（含逐请求明细表）

报告内容：

- **Test Parameters**：本次测试使用的全部参数（API Key 脱敏为 `***last4`）
- **Results**：与 TUI 结果页一致的指标，markdown 表格展示（PK 模式胜出值加粗，全部失败的 provider 显示 `N/A`）
- **Logs**：开启 Request Logging 时列出请求/响应 JSONL 日志文件名及丢弃/写错误警告

结果页底部显示 `Report saved: <文件名>`；写入失败仅显示警告，不影响结果展示。每次测试生成新文件，不会覆盖历史报告。手动 `ctrl+e` 纯文本导出功能不受影响。
```

- [ ] **Step 3: Update docs/requirements.md**

Append to the feature table (after F-104):

```markdown
| F-105 | 压测完成后自动将测试参数与结果写入当前目录的 markdown 文件（`bench_report_<时间戳>.md`，PK 合并为单个文件，结果用 markdown 表格展示） | ✅ 已实现 |
| F-106 | Prompt Cache Hit 测试完成后自动写入 `cache_hit_report_<时间戳>.md`，含逐请求明细表 | ✅ 已实现 |
| F-107 | markdown 报告包含脱敏的测试参数（API Key `***last4`）；开启 Request Logging 时包含日志文件名；结果页显示 `Report saved: <文件名>` 或写失败警告 | ✅ 已实现 |
```

- [ ] **Step 4: Full verification**

Run:

```bash
go build -o embedding_benchmark .
go test ./... -v
go vet ./...
```

Expected: build succeeds, all tests pass (bench + tui packages), no vet issues.

- [ ] **Step 5: Commit**

```bash
git add AGENTS.md README.md docs/requirements.md
git commit -m "docs: document auto markdown report feature"
```

---

## Self-Review Notes

- **Spec coverage**: auto-write on both triggers (Tasks 6–7), file naming/location (Task 5), params incl. mode-dependent rows (Tasks 1–3), all result sections mirroring TUI (Tasks 1–4), PK bold winner + N/A + notes (Task 3), Logs section (Task 2), cache-hit per-request table (Task 4), key redaction + escaping (Task 1), TUI status line (Tasks 6–7), write-failure warning path (Tasks 6–7 tests), `ctrl+e` unchanged (no task touches `export.go` or `plainText()`), docs (Task 8).
- **Type consistency**: `mdWriteCompletionSingle`/`mdWriteEmbeddingPK`/`mdWriteCompletionPK` stubs from Task 1 are replaced with identical signatures in Tasks 2–3. `markdownReportDir` is defined in Task 6 and reused in Task 7. `renderReportStatus` is defined in Task 6 (results_screen.go) and reused in Task 7 (cache_hit_results.go, same package). `mdAPIUsageTokens(r, value)` takes the value directly; `mdPKAPIUsageTokens(r, fn)` takes a selector func — signatures differ intentionally (single has the report, PK may have nil).
- **Known limitation (accepted in spec)**: two runs completing within the same second would share a filename and the second overwrites the first; rerun requires returning to the config screen so this is practically impossible.
