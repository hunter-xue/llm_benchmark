# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build
go build -o embedding_benchmark .

# Run (launches TUI)
./embedding_benchmark

# Optional external BPE file override
./embedding_benchmark --bpe-file /path/to/cl100k_base.tiktoken
```

## Architecture

The project is a TUI benchmark tool for OpenAI-compatible embedding/chat completion APIs and Anthropic Messages APIs, built with [bubbletea](https://github.com/charmbracelet/bubbletea).

### Package Layout

```
main.go                         -- Entry point: init tiktoken, set program ref, launch TUI
internal/
  bench/
    config.go                   -- ProviderConfig, BenchConfig, DetectMode, NormalizeURL
    error_classification.go     -- ClassifyError + stable error category constants
    tiktoken.go                 -- offlineOnlyBpeLoader, InitTiktoken, GenerateMeaningfulTextByTokens, GenerateTextByTokens fallback
    meaningful_sentences.txt    -- Embedded sentence pool for natural benchmark prompts
    stats.go                    -- percentile, average, EmbeddingReport, CompletionReport structs
    embedding.go                -- RunEmbeddingBench (concurrent, returns report)
    completion.go               -- RunCompletionBench + doCompletionRequest (streaming SSE)
    anthropic.go                -- RunAnthropicMessagesBench + Anthropic SSE parsing + request logging
    openloop.go                 -- RunOpenLoopCompletionBench (Poisson arrival; OpenAI or Anthropic by Mode)
    cache_hit.go                -- RunCacheHitTest (Completions cached_tokens / Anthropic cache_read+cache_creation)
    report_markdown.go          -- Markdown report generation (MarkdownBenchReport, MarkdownCacheHitReport) + WriteMarkdownReport
    request_log.go              -- Async request/response JSONL logger
  tui/
    app.go                      -- Root Model, screen state machine, global prog var
    styles.go                   -- lipgloss style constants
    keys.go                     -- Key binding definitions
    messages.go                 -- ProgressMsg, BenchDoneMsg custom tea.Msg types
    mode_select.go              -- Screen: choose embedding vs completion
    test_mode_select.go         -- Screen: single provider vs PK mode
    config_screen.go            -- Screen: parameter input form (textinput fields)
    running_screen.go           -- Screen: progress bars, spinner, async bench dispatch
    results_screen.go           -- Screen: results table in scrollable viewport, PK comparison with green winner
    single_response_config.go   -- Screen: one-off response request config
    single_response.go          -- Screen: one-off response viewport
    response_compare_config.go  -- Screen: two-provider response compare config
    response_compare.go         -- Screen: side-by-side response viewports
    cache_hit_config.go         -- Screen: prompt cache hit test config
    cache_hit_results.go        -- Screen: prompt cache hit result table
    error_viewport.go           -- Overlay: scrollable error log (bubbles/viewport)
```

### Screen Flow

```
ModeSelect -> TestModeSelect -> ConfigScreen -> RunningScreen -> ResultsScreen
                                     ^                                |
                                     +--------- press 'r' -----------+
```

### Benchmark Modes

- **Embedding mode**: Fires `POST /v1/embeddings` requests concurrently, measures E2E latency.
- **Completion mode**: Fires streaming `POST /v1/chat/completions` requests, measures TTFT, TPOT, E2E latency, TPM via SSE parsing, and reports API-returned `usage` token totals when available.
- **Anthropic Messages mode**: Fires streaming `POST /v1/messages` requests with Anthropic SSE parsing and reports API-returned input/output usage when available.

### Test Modes

- **Single provider**: Benchmark one API endpoint.
- **PK mode**: Benchmark two providers simultaneously with the same parameters. Results shown side by side with the winner (better metric) highlighted in green.
- **Single Response View**: Send one non-streaming prompt to one completion-like provider and inspect headers plus raw JSON.
- **Response Compare**: Send the same non-streaming prompt to two completion-like providers and compare headers plus raw JSON side by side.
- **Prompt Cache Hit Test**: Repeat one user-entered prompt and report cache hits. Chat Completions reads `usage.prompt_tokens_details.cached_tokens`; Anthropic Messages reads `cache_read_input_tokens` / `cache_creation_input_tokens` (and auto-injects top-level `cache_control` unless Custom Params already set it).

### Key Design Decisions

- **Token counting**: Uses `tiktoken-go` with the embedded `cl100k_base` encoding (offline only — `offlineOnlyBpeLoader` blocks network downloads). `--bpe-file` optionally overrides the embedded BPE file.
- **Benchmark input generation**: Main benchmark modes generate exact-length prompts from the embedded natural sentence pool. `GenerateMeaningfulTextByTokens` strictly matches `TargetTokens`; `GenerateTextByTokens` remains as a fallback.
- **API usage reporting**: Completion reports keep local tiktoken-based performance counters and separately aggregate API-returned raw token usage fields (`APIPromptTokens`, `APICompletionTokens`, `APITotalTokens`, `APIUsageCount`, `MissingAPIUsageCount`). OpenAI-compatible streaming requests default to `stream_options.include_usage=true`; Custom Params can override it. Missing usage is shown as `N/A`, never counted as zero.
- **Concurrency model**: Two modes via Load Model toggle (Chat Completion and Anthropic Messages): closed-loop (default) or open-loop (Poisson arrival + Max In-Flight semaphore + QueueTime).
- **Progress reporting**: Benchmark goroutines call `prog.Send(ProgressMsg{...})` where `prog` is a package-level `*tea.Program` set before `p.Run()`.
- **Error log**: Raw errors are collected in `ErrorDetails map[string]int`; stable categories are counted in `ErrorCategories map[string]int`. Press `e` during/after benchmark to view category summaries plus raw details in a scrollable viewport overlay.

### Key Types in `internal/bench`

```go
type ProviderConfig struct { Name, URL, APIKey, Model string }
type BenchConfig struct { Mode, Concurrency, TotalRequests, TargetTokens, MaxOutputTokens int; SystemPrompt string }
type EmbeddingReport struct { ...; ErrorDetails, ErrorCategories map[string]int; Valid bool }
type CompletionReport struct { ...; APIPromptTokens, APICompletionTokens, APITotalTokens, APIUsageCount, MissingAPIUsageCount int; ErrorDetails, ErrorCategories map[string]int; Valid bool }
```

`Valid == false` means all requests failed (shown as "N/A" in PK mode comparison).

### Key Flags

| Flag | Default | Notes |
|------|---------|-------|
| `--bpe-file` | empty | Optional external BPE override |

All other benchmark parameters (URL, key, model, concurrency, requests, tokens, etc.) are entered interactively in the TUI.

### Validation & Error Behaviour

- URL, concurrency, total requests, and input tokens are validated before starting.
- Non-200 HTTP responses include up to 512 bytes of body in the error string.
- Benchmark errors are classified by `ClassifyError` into stable categories such as `rate_limit`, `auth`, `bad_request`, `server_error`, `timeout`, `connection`, `empty_output`, `stream_error`, `parse_error`, `client_error`, and `other`.
- The `percentile` function uses the nearest-rank method: `idx = ceil(n × p) − 1`.
- `SkippedChunks` counts malformed SSE JSON lines, reported as a warning.
