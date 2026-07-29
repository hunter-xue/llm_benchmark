# Auto Markdown Report — Design Spec

## Overview

When a benchmark-style test finishes and its results screen is shown, the tool automatically writes a markdown report file into the current working directory. The file records the test parameters used and the test results (rendered as markdown tables), plus the request-log file names when Request Logging was enabled. No key press is required; every run produces exactly one report file.

## Background

Today results only live in the TUI. The existing `ctrl+e` export dumps ANSI-stripped screen text to a user-named file — it is manual, contains no structured parameter section, and is not a real markdown document. Users who run repeated benchmarks want a durable, diffable, shareable record of *parameters + results* per run.

## Scope

- **In scope (auto-write)**:
  - Benchmark results screen — Embedding and Chat Completion, single provider and PK mode (Anthropic Messages mode shares the Chat Completion results screen and is therefore covered the same way).
  - Prompt Cache Hit results screen.
- **Out of scope**: Single Response View and Response Compare (raw response viewers; their manual `ctrl+e` export is unchanged).
- The existing `ctrl+e` plain-text export is unchanged on all screens.

## File Naming & Location

- Written to the current working directory.
- Benchmark runs: `bench_report_YYYYMMDD_HHMMSS.md`
- Cache Hit runs: `cache_hit_report_YYYYMMDD_HHMMSS.md`
- Timestamp is local time at report generation; one file per run (PK mode produces a single combined file). Reruns (`r`) produce new files; nothing is overwritten.

## Trigger Points

- `app.go` `BenchDoneMsg` handler: when `doneProviders >= len(m.providers)` (the moment the UI transitions to `ScreenResults`), generate and write the report once, using the final accumulated reports.
- `app.go` `CacheHitDoneMsg` handler: generate and write once.
- The write is a small synchronous one-shot file write in the Update handler. It happens after the benchmark has fully finished, so it cannot affect any measured metric.
- If the user cancels a run mid-way (`esc` on the running screen), not all providers report done, so no file is written.
- Write failure is recorded and shown as a warning line on the results screen; result display is unaffected.

## Markdown Generation (`internal/bench/report_markdown.go`)

New file in the `bench` package. Generation is a pure data-to-string transform, independently unit-testable without the TUI:

```go
// MarkdownBenchReport renders a benchmark report (embedding or completion,
// single or PK). Exactly one of embReports/compReports is non-empty.
func MarkdownBenchReport(apiMode, testMode string, providers []ProviderConfig, cfg BenchConfig, embReports []*EmbeddingReport, compReports []*CompletionReport, now time.Time) string

// MarkdownCacheHitReport renders a Prompt Cache Hit test report.
func MarkdownCacheHitReport(provider ProviderConfig, cfg CacheHitConfig, userPrompt string, r *CacheHitReport, now time.Time) string

// WriteMarkdownReport writes content to dir/<prefix>_YYYYMMDD_HHMMSS.md
// (timestamp from now) and returns the file name.
func WriteMarkdownReport(dir, prefix, content string, now time.Time) (string, error)
```

### Credential redaction

Provider API keys are written redacted: `***<last 4 chars>`; keys of 4 chars or fewer become `***` (same rule as request logging). Empty key → `(none)`.

### Escaping

`|` inside any cell value (custom params, error summaries, prompts) is escaped as `\|`; newlines inside values are replaced with spaces.

## File Structure

### Benchmark report (single provider)

```markdown
# Benchmark Report — Chat Completion

Generated: 2026-07-29 15:30:45

## Test Parameters

| Parameter | Value |
|---|---|
| API Mode | completion |
| Test Mode | single |
| Provider | my-server |
| URL | http://127.0.0.1:3000/v1/chat/completions |
| Model | qwen3 |
| API Key | ***wxyz |
| Concurrency | 10 |
| Total Requests | 100 |
| Input Tokens | 1000 |
| Max Output Tokens | 512 |
| Load Model | closed_loop |
| TTFT Includes Reasoning | true |
| Request Logging | true |
| System Prompt | You are... |
| Custom Params | {"temperature":0} |

## Results

| Metric | Value |
|---|---|
| Total Requests | 100 |
| Successful | 98 (98.0%) |
| Failed | 2 |
| Error Categories | timeout: 2 |

| Metric | Value |
|---|---|
| Wall Time | 12.34 s |
| RPS | 8.10 |
| ... (same sections, names, and order as the TUI results screen) |
```

- Parameter rows are mode-dependent: embedding omits completion-only rows; open-loop replaces the `Concurrency` row with `Request Rate` and `Max In-Flight` rows (in `BenchConfig` the open-loop `Concurrency` field only mirrors `MaxInFlight`, so showing it would be redundant); Anthropic Messages mode renders like closed-loop Chat Completion but omits the `Load Model` and `Request Logging` rows (it has neither toggle); `System Prompt` / `Custom Params` rows are omitted when empty.
- Result sections mirror the TUI exactly: summary (+ error categories) → throughput → API usage → TTFT → TPOT → E2E → Queue Time (open-loop only, same `> 0` condition as the TUI) → skipped-chunks warning row.
- When `Valid == false`: metric sections are replaced by a single line `_All requests failed — no metrics available._`; the summary table is still shown.
- When Request Logging was on (`LogRequestsFile != ""`), a final section is added:

```markdown
## Logs

- Requests: `bench_requests_20260729-153045.jsonl`
- Responses: `bench_responses_20260729-153045.jsonl`
- ⚠ 3 log entries dropped (disk too slow)   <!-- only when LogDroppedCount > 0 -->
- ⚠ log write error: ...                     <!-- only when LogError != "" -->
```

### Benchmark report (PK)

- Title: `# Benchmark Report — Chat Completion (PK)`.
- Parameters table becomes three columns: `| Parameter | <nameA> | <nameB> |` for provider-specific rows (Provider/URL/Model/API Key/Custom Params); shared parameters stay in a two-column table.
- Results table: `| Metric | <nameA> | <nameB> |`, same rows and order as the TUI PK view (Success/Total, Error Categories, API usage rows, metric groups with blank separator rows omitted). The winning cell is **bold**; invalid provider cells are `N/A`; ties are not bolded.
- A provider with `Valid == false` gets a note line: `_<name>: all requests failed_`.

### Cache Hit report

```markdown
# Prompt Cache Hit Report

Generated: ...

## Test Parameters

| Parameter | Value |
|---|---|
| Provider / URL / Model / API Key | ... |
| Test Count | 5 |
| Max Output Tokens | 1 |
| Interval | 2s |
| System Prompt | ... (omitted when empty) |
| User Prompt | ... |

## Results

| Metric | Value |
|---|---|
| Total Requests / Successful / Failed / Wall Time | ... |
| Error Categories | ... (only when present) |

| Metric | Value |
|---|---|
| Prompt Tokens | 12345 |
| Cached Tokens | 12000 |
| Overall Hit Rate | 97.20% |
| Avg Request Hit Rate | 96.80% |
| Avg Latency | 123.45 ms |
| Missing Usage / Missing Cached Field | ... (only when > 0) |

## Per-Request Results

| # | Latency | Prompt | Cached | Hit Rate |
|---|---|---|---|---|
| 1 | 120.00 ms | 4096 | 4096 | 100.00% |
| 2 | error: connection refused | | | |
```

- `Valid == false`: metrics table replaced by `_All requests failed — no cache metrics available._`; per-request table still shown.

## TUI Feedback

- `resultsModel`: new fields `reportFile, reportErr string`, set by the app after the write attempt. Rendered as one dim line between content and hints: `Report saved: bench_report_20260729_153045.md`, or an error-styled `⚠ failed to write report: <err>`. No new keybindings; hints unchanged.
- `cacheHitResultsModel`: same two fields and status line.
- PK mode: the single combined file name is shown.

## Error Handling

- Any `os.WriteFile` / directory error is captured into `reportErr` and shown on the results screen; the TUI continues normally.
- Markdown generation itself cannot fail (pure string building).

## Testing

New `internal/bench/report_markdown_test.go`:

| Case | Assertion |
|---|---|
| Embedding single | params table contains provider/URL/model/concurrency; results contain RPS/latency rows; redacted key |
| Completion single | API usage rows, TTFT/TPOT/E2E sections, Queue Time section present only when queue metrics > 0 |
| Completion single + logging | `## Logs` section lists both file names; dropped-count and write-error warnings rendered |
| Completion PK | three-column table, winner cell bold, invalid provider → `N/A` + note line |
| All requests failed | `_All requests failed_` line, no metric sections, summary still present |
| Cache hit | params (incl. User Prompt), metrics, per-request rows incl. error row and N/A cached cells |
| Key redaction | last-4 kept; ≤4-char key → `***`; empty → `(none)` |
| Escaping | value containing `|` renders as `\|` |
| WriteMarkdownReport | creates `<prefix>_<ts>.md` in `t.TempDir()` with exact content |

TUI: extend results screen tests to assert the `Report saved:` line appears when `reportFile` is set.

## Documentation Updates

- `AGENTS.md`: add `report_markdown.go` to the package layout and a bullet under Key Design Decisions describing auto report writing.
- `README.md`: feature list + results-screen description.
- `docs/requirements.md`: new feature row for auto markdown report.
