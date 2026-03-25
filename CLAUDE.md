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

The project is a TUI benchmark tool for OpenAI-compatible embedding and chat completion APIs, built with [bubbletea](https://github.com/charmbracelet/bubbletea).

### Package Layout

```
main.go                         -- Entry point: init tiktoken, set program ref, launch TUI
internal/
  bench/
    config.go                   -- ProviderConfig, BenchConfig, DetectMode, NormalizeURL
    tiktoken.go                 -- offlineOnlyBpeLoader, InitTiktoken, GenerateTextByTokens
    stats.go                    -- percentile, average, EmbeddingReport, CompletionReport structs
    embedding.go                -- RunEmbeddingBench (concurrent, returns report)
    completion.go               -- RunCompletionBench + doCompletionRequest (streaming SSE)
  tui/
    app.go                      -- Root Model, screen state machine, global prog var
    styles.go                   -- lipgloss style constants
    keys.go                     -- Key binding definitions
    messages.go                 -- ProgressMsg, BenchDoneMsg custom tea.Msg types
    mode_select.go              -- Screen: choose embedding vs completion
    test_mode_select.go         -- Screen: single provider vs PK mode
    config_screen.go            -- Screen: parameter input form (textinput fields)
    running_screen.go           -- Screen: progress bars, spinner, async bench dispatch
    results_screen.go           -- Screen: results table, PK comparison with green winner
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
- **Completion mode**: Fires streaming `POST /v1/chat/completions` requests, measures TTFT, TPOT, E2E latency, TPM via SSE parsing.

### Test Modes

- **Single provider**: Benchmark one API endpoint.
- **PK mode**: Benchmark two providers simultaneously with the same parameters. Results shown side by side with the winner (better metric) highlighted in green.

### Key Design Decisions

- **Token counting**: Uses `tiktoken-go` with `cl100k_base` encoding (offline only — `offlineOnlyBpeLoader` blocks network downloads). The BPE file must exist locally.
- **Concurrency model**: Buffered `taskQueue` channel pre-filled with N tasks; `concurrency` goroutines drain it.
- **Progress reporting**: Benchmark goroutines call `prog.Send(ProgressMsg{...})` where `prog` is a package-level `*tea.Program` set before `p.Run()`.
- **Error log**: All errors are collected in `ErrorDetails map[string]int`. Press `e` during/after benchmark to view in a scrollable viewport overlay.

### Key Types in `internal/bench`

```go
type ProviderConfig struct { Name, URL, APIKey, Model string }
type BenchConfig struct { Mode, Concurrency, TotalRequests, TargetTokens, MaxOutputTokens int; SystemPrompt string }
type EmbeddingReport struct { ...; Valid bool }
type CompletionReport struct { ...; Valid bool }
```

`Valid == false` means all requests failed (shown as "N/A" in PK mode comparison).

### Key Flags

| Flag | Default | Notes |
|------|---------|-------|
| `--bpe-file` | `./cl100k_base.tiktoken` | Must exist locally |

All other benchmark parameters (URL, key, model, concurrency, requests, tokens, etc.) are entered interactively in the TUI.

### Validation & Error Behaviour

- URL, concurrency, total requests, and input tokens are validated before starting.
- Non-200 HTTP responses include up to 512 bytes of body in the error string.
- The `percentile` function uses the nearest-rank method: `idx = ceil(n × p) − 1`.
- `SkippedChunks` counts malformed SSE JSON lines, reported as a warning.
