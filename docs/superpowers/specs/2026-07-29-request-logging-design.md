# Chat Completion Request Logging — Design Spec

## Overview

Add an opt-in `Request Logging` toggle to the Chat Completion **single provider** benchmark config. When ON (default OFF), every benchmark request's complete HTTP request (headers + exact wire body) and complete HTTP response (status, headers, raw SSE stream lines, or error) are written asynchronously to two JSONL files in the current directory. Each request carries a locally-generated request ID that correlates its request and response log entries. Logging is fully asynchronous and must never delay any measured metric (TTFT / E2E / TPOT / QueueTime) or back-pressure the benchmark.

## Background

When benchmarking LLM serving stacks, users often need to inspect exactly what was sent and received — e.g. to verify request payloads, debug unexpected outputs, or compare provider behavior offline. Today the tool only keeps aggregated metrics; the wire data is lost. Because this is a performance measurement tool, logging must be strictly best-effort: **it is acceptable to drop log entries, never acceptable to slow down the benchmark.**

## Scope

- **API Mode**: Chat Completion only (`ModeCompletion`). Embedding and Anthropic Messages modes do not get the toggle.
- **Test Mode**: Single Provider benchmark only. PK mode does not get the toggle. Single Response View, Response Compare, and Prompt Cache Hit Test are unaffected.
- **Load Models**: Closed-loop (`RunCompletionBench`) and open-loop (`RunOpenLoopCompletionBench`) are both covered (both call `doCompletionRequest`).

## Request ID

- Each benchmark run generates a **run ID** = local start timestamp formatted `20060102-150405` (e.g. `20260729-153045`).
- Each request ID = `<runID>-<seq>` where `seq` is a zero-padded 6-digit per-runner atomic counter starting at 1 (e.g. `20260729-153045-000123`).
- The ID is **local only** — written to the log files, never sent to the server (no extra headers, no body fields).

## Log Files

Created in the current working directory at benchmark start (only when the toggle is ON):

- `bench_requests_<runID>.jsonl` — one line per request
- `bench_responses_<runID>.jsonl` — one line per request

### Request entry

`body` is the exact wire bytes after `MergeCustomParams` (embedded as `json.RawMessage`, never re-serialized):

```json
{
  "request_id": "20260729-153045-000123",
  "ts": "2026-07-29T15:30:45.123Z",
  "method": "POST",
  "url": "https://api.openai.com/v1/chat/completions",
  "headers": {
    "Content-Type": "application/json",
    "Accept": "text/event-stream",
    "Authorization": "Bearer ***wxyz"
  },
  "body": { "model": "gpt-4o-mini", "messages": [ ... ], "stream": true }
}
```

### Response entry (200 OK, streaming)

`chunks` holds every **non-blank raw SSE line** exactly as read from the stream, including the `data:` prefix, comment lines (e.g. `: ping`), and `data: [DONE]`. Blank event-delimiter lines are skipped (they carry no information).

```json
{
  "request_id": "20260729-153045-000123",
  "ts": "2026-07-29T15:30:46.005Z",
  "duration_ms": 842,
  "status": "200 OK",
  "headers": { "Content-Type": "text/event-stream" },
  "chunks": ["data: {\"id\":\"...\"}", "data: [DONE]"],
  "error": ""
}
```

### Response entry (failure)

- HTTP error (non-200): `status` + `headers` + `body` (error response body, capped at 512 bytes, same limit as existing error handling) + `error` message. `chunks` empty.
- Connection / stream errors: only `error` (+ partial `chunks` if the stream died mid-way).

## Credential Redaction

The `Authorization` header value is redacted before logging: `Bearer ***<last 4 chars>` (e.g. `Bearer ***wxyz`). The last 4 characters are kept so logs can be correlated with the key in use. Keys of 4 characters or fewer are fully redacted as `Bearer ***` (nothing revealed). No other headers contain secrets in this tool (Custom Params only affect the body). Response headers are logged verbatim.

## Async Architecture (Metric Isolation)

### Core Principle

Logging calls never appear inside a measured timing window, never block a request goroutine, and do no serialization/IO outside the writer goroutine.

### Operation order in doCompletionRequest

```
build body → build http.Request → LogRequest() → sendTime = now → client.Do
→ SSE scan loop → lastTokenTime recorded → LogResponse()
```

- `LogRequest` runs **before** `sendTime` is captured; `LogResponse` runs **after** `lastTokenTime` is captured. TTFT / E2E / TPOT / QueueTime measurement code paths are unchanged.
- Inside the SSE loop the only added work (when logging is enabled) is `chunks = append(chunks, line)` — `line` is already allocated by `scanner.Text()`; the append copies a 16-byte string header. Nanoseconds vs. network milliseconds.

### RequestLogger (internal/bench/request_log.go, new)

- `NewRequestLogger(dir, runID string) (*RequestLogger, error)` creates both files (`O_CREATE|O_WRONLY|O_APPEND`). `dir` is `.` in production; tests pass `t.TempDir()`. The runners pass an unexported package-level variable `requestLogDir` (default `"."`) so tests can redirect output without changing public signatures.
- Internals: buffered channel (capacity 4096) + single writer goroutine. `LogRequest` / `LogResponse` are safe for concurrent use; both delegate to an unexported `enqueue(entry)` method.
- **Non-blocking enqueue**: `select { case ch <- entry: default: atomic dropped++ }`. A full channel (slow disk) drops the entry — the benchmark never waits.
- Request goroutines hand over raw data references (body bytes, header map, chunk slice). **JSON marshaling, redaction of the header map copy, bufio writes all happen in the writer goroutine.** (Request headers are cloned before handoff since the map is shared; the clone happens pre-`sendTime`.)
- Writer goroutine batches flushes: flush every 64 entries or 500 ms, whichever comes first.
- `Close()` closes the channel, drains remaining entries, flushes, closes files, and waits for the writer goroutine.
- `DroppedCount()` returns the atomic drop counter; `Err()` returns the first write error, if any.

### Failure policy

| When | Behavior |
|------|----------|
| File creation fails at benchmark start | Benchmark does not start; runner immediately returns a report with `ErrorDetails{"failed to initialize request logging: ...": 1}`, `Valid = false` (mirrors the `prepareBenchText` failure path) |
| Write fails mid-run | Writer records the first error, keeps draining; affected entries are lost. Reported via `CompletionReport.LogError` + `LogDroppedCount`. Benchmark continues unaffected |

## Config Changes

### BenchConfig (internal/bench/config.go)

One new field:

```go
type BenchConfig struct {
    // ... existing fields unchanged ...

    RequestLogging bool // chat completion single-provider: log request/response to JSONL files
}
```

Zero value `false` — all other construction paths (compare, cache-hit, anthropic) are unaffected automatically.

### CompletionReport (internal/bench/stats.go)

Four new fields (empty/zero = logging disabled):

```go
LogRequestsFile  string // path of the requests JSONL file
LogResponsesFile string // path of the responses JSONL file
LogDroppedCount  int    // entries dropped because the disk could not keep up
LogError         string // first log write error, if any
```

## Runner Wiring

- `RunCompletionBench` / `RunOpenLoopCompletionBench`: when `cfg.RequestLogging` is true, create the logger at start (`NewRequestLogger(".", runID)`), `defer Close()`, fill the four report fields before returning. Per-request IDs come from a local atomic counter.
- `doCompletionRequest` gains two parameters: `reqID string, logger *RequestLogger`. `logger == nil` disables logging entirely (one branch per SSE line, no allocations) — PK mode and any future caller pass `nil`.
- No changes to `RunEmbeddingBench`, `RunAnthropicMessagesBench`, `DoCompareRequest`, or the cache-hit path.

## TUI Changes

### Config Screen (internal/tui/config_screen.go)

- New toggle field **`Request Logging`**, rendered `( ) off  (●) on` / `(●) off  ( ) on`, **default OFF**.
- Shown only when `apiMode == "completion"` **and** single provider (not PK). Position: last field, after "System Prompt".
- Follows the existing toggle idioms: state `requestLoggingOn bool` on `configModel` (set in `newConfigModel`, survives `rebuildFields()`), dispatch in `toggleFocusedField()` by label, renderer in `renderToggle()` — both use the panic-on-unknown-label convention.
- `validate()` sets `cfg.RequestLogging = m.requestLoggingOn` for the applicable case.

### Results Screen (internal/tui/results_screen.go)

When logging was enabled (report's `LogRequestsFile` non-empty):

- Dim line under the provider's results: `Logs: bench_requests_20260729-153045.jsonl / bench_responses_20260729-153045.jsonl`
- If `LogDroppedCount > 0` or `LogError` non-empty: warning-styled line, e.g. `⚠ 12 log entries dropped (disk too slow)` / `⚠ log write error: ...`.

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Toggle OFF | No files created, no IDs generated, hot path gains one `nil` check per request |
| Request fails before send (marshal error) | Nothing logged (no request ID consumed is fine — IDs need not be gapless) |
| HTTP non-200 | Response entry with status/headers/body(≤512B)/error, no chunks |
| Stream dies mid-way | Response entry with partial chunks + error |
| Context cancelled mid-request | Same as stream error: partial chunks + error logged |
| Two runs within the same second | Files opened with `O_APPEND`; entries stay ordered by ts; acceptable |
| Very large response (many MB) | Chunks buffered in memory for that request until it completes — bounded by single response size, acceptable |
| Run started in a read-only directory | File creation fails → benchmark does not start, error surfaced (see Failure policy) |

## Testing

### Unit Tests — internal/bench/request_log_test.go (new)

| Test | What It Verifies |
|------|-----------------|
| End-to-end logged run | `httptest.Server` SSE endpoint + `RunCompletionBench` with `RequestLogging: true` (test overrides the unexported `requestLogDir` package var with `t.TempDir()`): both files exist, each line parses as JSON, request/response `request_id`s match 1:1, request `body` bytes equal the exact wire body, `chunks` contain every `data:` line incl. `[DONE]`, `Authorization` is redacted with last-4 preserved |
| Non-200 response | Response entry has status, headers, body snippet, error; no chunks |
| Toggle OFF | No files created |
| Non-blocking drop | Construct `RequestLogger` directly (same-package test) with a full channel; `enqueue` returns immediately and increments the drop counter |
| Redaction | `Bearer sk-abcdefgh` → `Bearer ***efgh`; missing/short keys handled without panic |

### Config Screen Tests — internal/tui/config_screen_test.go (additions)

| Test | What It Verifies |
|------|-----------------|
| Toggle visible | Present in fieldDefs for Chat Completion single provider |
| Toggle hidden | Absent in PK mode, Embedding, Anthropic Messages |
| Default | `requestLoggingOn == false`; `validate()` injects `cfg.RequestLogging == false`; flipped state injects `true` |

## File Changes Summary

| File | Change |
|------|--------|
| `internal/bench/request_log.go` | **New** — RequestLogger, entry types, redaction, async writer |
| `internal/bench/request_log_test.go` | **New** — logger + wiring tests |
| `internal/bench/config.go` | Add `RequestLogging` to BenchConfig |
| `internal/bench/stats.go` | Add 4 log fields to CompletionReport |
| `internal/bench/completion.go` | `doCompletionRequest` new params; log request/response; runner creates logger + IDs |
| `internal/bench/openloop.go` | Same logger + ID wiring in `RunOpenLoopCompletionBench` |
| `internal/tui/config_screen.go` | New toggle field, state, rendering, validation |
| `internal/tui/config_screen_test.go` | Toggle tests |
| `internal/tui/results_screen.go` | Show log paths + drop/write-error warning |
| `.gitignore` | Add `bench_requests_*.jsonl` / `bench_responses_*.jsonl` |
| `AGENTS.md` | Document request logging design decision |

## Non-Goals (YAGNI)

- No logging in PK mode, Embedding, Anthropic Messages, Single Response View, Response Compare, or Cache Hit Test
- Request ID is not sent to the server (no header, no body field)
- No assembled/plain-text copy of the response in the log (raw SSE chunks only)
- No log rotation, compression, or retention management
- No request-body size limits (benchmark payloads are self-generated and bounded by config)
