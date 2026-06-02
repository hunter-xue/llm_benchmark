# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build
go build -o embedding_benchmark .

# Run (launches TUI)
./embedding_benchmark

# Custom BPE file path
./embedding_benchmark --bpe-file /path/to/cl100k_base.tiktoken
```

## Architecture

The project is a TUI benchmark tool for OpenAI-compatible and Anthropic APIs, built with [bubbletea](https://github.com/charmbracelet/bubbletea).

### Package Layout

```
main.go                         -- Entry point: init tiktoken, set program ref, launch TUI
                                    Also defines CLI subcommands (single, pk, compare, cache-hit)
internal/
  bench/
    anthropic.go               -- Anthropic Messages API: streaming bench + compare request
    cache_hit.go                -- Prompt cache hit test: repeated non-streaming requests
    completion.go               -- OpenAI completion bench + compare request (streaming SSE)
    config.go                   -- ProviderConfig, BenchConfig, Mode constants, NormalizeURL
    serialize.go                -- All output types, BenchmarkResult, unified JSON/XML serialization
    tiktoken.go                 -- offlineOnlyBpeLoader, InitTiktoken, GenerateTextByTokens
    stats.go                    -- percentile, average, EmbeddingReport, CompletionReport structs
    embedding.go                -- RunEmbeddingBench (concurrent, returns report)
  tui/
    app.go                      -- Root Model, screen state machine, global prog var
    styles.go                   -- lipgloss style constants
    keys.go                     -- Key binding definitions
    messages.go                 -- ProgressMsg, BenchDoneMsg, CompareResponseMsg, etc.
    mode_select.go              -- Screen: choose embedding / completion / anthropic
    test_mode_select.go         -- Screen: choose test mode (single, PK, compare, etc.)
    config_screen.go            -- Screen: benchmark parameter input form
    running_screen.go           -- Screen: progress bars, spinner, async bench dispatch
    results_screen.go           -- Screen: results table, PK comparison with green winner
    error_viewport.go           -- Overlay: scrollable error log (bubbles/viewport)
    response_compare_config.go  -- Screen: Response Compare config form (two providers + user message)
    response_compare.go         -- Screen: side-by-side headers+body viewport comparison
    single_response_config.go   -- Screen: Single Response View config form (one provider + user message)
    single_response.go          -- Screen: single viewport showing raw JSON response
    cache_hit_config.go         -- Screen: Prompt Cache Hit config form
    cache_hit_results.go        -- Screen: cache hit rate results table
    export.go                   -- Overlay: filename input + file write for result export
```

### Screen Flow

```
ModeSelect -> TestModeSelect -> ConfigScreen -> RunningScreen -> ResultsScreen
                                     ^                                |
                                     +--------- press 'r' -----------+

TestModeSelect -> ResponseCompareConfig -> ResponseCompare
TestModeSelect -> SingleResponseConfig  -> SingleResponse
TestModeSelect -> CacheHitConfig        -> CacheHitRunning -> CacheHitResults
```

### API Modes

- **Embedding mode**: Fires `POST /v1/embeddings` requests concurrently, measures E2E latency.
- **Completion mode**: Fires streaming `POST /v1/chat/completions` requests, measures TTFT, TPOT, E2E latency, TPM via SSE parsing.
- **Anthropic mode**: Fires streaming `POST /v1/messages` requests using the Anthropic Messages API with `x-api-key` auth and `anthropic-version: 2023-06-01` header. Reuses `CompletionReport` since metrics are identical.

Mode constants: `ModeEmbedding = "embedding"`, `ModeCompletion = "completion"`, `ModeAnthropic = "anthropic"`.

### Test Modes

- **Single provider**: Benchmark one API endpoint.
- **PK mode**: Benchmark two providers simultaneously with the same parameters. Results shown side by side with the winner (better metric) highlighted in green.
- **Response Compare**: Send one non-streaming request to two providers with the same user message, compare full HTTP headers + body side by side in scrollable viewports. Available for completion and anthropic modes only.
- **Single Response View**: Send one non-streaming request to a single provider, view the raw JSON response in a scrollable viewport. Available for completion and anthropic modes only.
- **Cache Hit Test**: Send repeated non-streaming Chat Completions requests to measure prompt cache hit rate from `usage.prompt_tokens_details.cached_tokens`. Available for completion mode only.

### CLI Subcommands

All test modes are also available as CLI subcommands:

| Subcommand | Maps to |
|-------------|---------|
| `single` | Single provider benchmark |
| `pk` | PK mode benchmark |
| `compare` | Response Compare |
| `cache-hit` | Cache Hit Test |

### Key Design Decisions

- **Token counting**: Uses `tiktoken-go` with `cl100k_base` encoding (offline only — `offlineOnlyBpeLoader` blocks network downloads). The BPE file must exist locally.
- **Concurrency model**: Buffered `taskQueue` channel pre-filled with N tasks; `concurrency` goroutines drain it.
- **Progress reporting**: Benchmark goroutines call `prog.Send(ProgressMsg{...})` where `prog` is a package-level `*tea.Program` set before `p.Run()`.
- **Error log**: All errors are collected in `ErrorDetails map[string]int`. Press `e` during/after benchmark to view in a scrollable viewport overlay.
- **Custom Params**: Optional JSON object (`--custom-params`) merged into every request body, allowing provider-specific parameters (e.g. temperature). Validated at config time via `validateCustomParams`.

### Key Types in `internal/bench`

```go
type ProviderConfig struct { Name, URL, APIKey, Model, CustomParams string }
type BenchConfig struct { Mode string; Concurrency, TotalRequests, TargetTokens, MaxOutputTokens int; SystemPrompt string }
type EmbeddingReport struct { ...; Valid bool }
type CompletionReport struct { ...; Valid bool }
type CacheHitReport struct { ...; Valid bool }
```

`Valid == false` means all requests failed (shown as "N/A" in PK mode comparison).

### Unified JSON Output Format

All subcommands produce output wrapped in `BenchmarkResult` with the same top-level structure:

```json
{
  "mode": "completion|embedding|anthropic",
  "test_mode": "single|pk|compare|cache_hit",
  "<test_mode>": { ... content ... }
}
```

The content key matches `test_mode` value:
- `"single"`: `EmbeddingOutput` or `CompletionOutput` (determined by `mode`)
- `"pk"`: `EmbeddingPKOutput` or `CompletionPKOutput` with `provider_a`/`provider_b` sub-objects
- `"compare"`: `CompareOutput` with `user_message`, `provider_a`/`provider_b` (each has `name`, `headers`, `body`, `error`), and `valid`
- `"cache_hit"`: `CacheHitOutput` with per-request `results` array and aggregate stats

`BenchmarkResult` uses custom `MarshalJSON`/`MarshalXML` to produce this structure (private fields + dynamic key dispatch).

**Compare body handling**: `CompareProviderResult.Body` is stored internally as a string (for TUI display), but `MarshalJSON` inlines it as a native JSON object when the content is valid JSON, so consumers can directly access `result["compare"]["provider_a"]["body"]` as a dict without `json.loads()`.

### Key Flags

| Flag | Default | Notes |
|------|---------|-------|
| `--bpe-file` | `./cl100k_base.tiktoken` | Must exist locally |
| `--mode` | (required) | `embedding`, `completion`, or `anthropic` |
| `--url` | (required) | API endpoint URL |
| `--api-key` | "" | API key |
| `--model` | (required) | Model name |
| `--name` | "Provider A" | Display name |
| `--custom-params` | "" | JSON object merged into request body |
| `--concurrency` | 1 | Concurrent requests |
| `--requests` | 10 | Total requests |
| `--tokens` | 128 | Target input tokens |
| `--max-output-tokens` | 256 | Max output tokens (0=unlimited; 0→4096 for anthropic) |
| `--system-prompt` | "" | System prompt for completion/anthropic |
| `--output` | `json` | Output format: `json` or `xml` |
| `--output-file` | "" | Write to file instead of stdout |

PK mode adds `--url-b`, `--api-key-b`, `--model-b`, `--name-b`, `--custom-params-b` for the second provider.

Compare mode uses `--url`/`--url-b` for providers and `--user-message` (required) instead of `--tokens`.

Cache-hit mode uses `--count` (default 5), `--interval` (default 3s), and `--user-prompt` (optional; auto-generated from `--tokens` if empty).

### Validation & Error Behaviour

- URL, concurrency, total requests, and input tokens are validated before starting.
- `CustomParams` is validated as a JSON object at config time.
- Non-200 HTTP responses include up to 512 bytes of body in the error string.
- The `percentile` function uses the nearest-rank method: `idx = ceil(n × p) − 1`.
- `SkippedChunks` counts malformed SSE JSON lines, reported as a warning.
