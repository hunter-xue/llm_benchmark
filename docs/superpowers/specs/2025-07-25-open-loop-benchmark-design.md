# Open-Loop Benchmark Mode — Design Spec

## Overview

Add an open-loop (SGLang-style) concurrency model to the existing Chat Completion benchmark. The current closed-loop model (fixed workers draining a task queue) is preserved unchanged. Users select between the two models via a radio toggle in the config screen.

## Background

The current benchmark uses a closed-loop concurrency model: N workers each send a request, wait for the response, then immediately pick up the next task. This means request rate is implicitly coupled to server latency.

The open-loop model decouples request generation from request execution. A single generator goroutine produces requests at a configurable rate (Poisson distribution). A semaphore limits the maximum number of in-flight requests. Requests that exceed the semaphore capacity queue and wait — measuring queue time is a key metric.

## Scope

- **API Mode**: Chat Completion only (Embedding and Anthropic Messages are not affected)
- **Test Modes**: Single Provider and PK Mode (both must use the same Load Model)
- **Distribution**: Poisson only (no uniform, no burst)
- **Queue timeout**: None (requests wait indefinitely, matching SGLang behavior)

## Config Changes

### BenchConfig (internal/bench/config.go)

Three new fields added:

```go
type BenchConfig struct {
    // ... existing fields unchanged ...

    LoadModel   string  // "closed_loop" (default) or "open_loop"
    RequestRate int     // open-loop: requests per second (Poisson rate)
    MaxInFlight int     // open-loop: max concurrent in-flight requests (semaphore)
}
```

- `LoadModel` defaults to `"closed_loop"`; zero-value behavior is identical to today
- `RequestRate` is an `int` (requests per second)
- `MaxInFlight` replaces `Concurrency` in open-loop mode; `Concurrency` is ignored

## Core Scheduler — internal/bench/openloop.go

New file containing `RunOpenLoopCompletionBench`.

### Architecture

```
Generator Goroutine (1)
  │
  ├─ for i in 0..TotalRequests:
  │    ├─ sleep(Poisson inter-arrival time)  // skip for i=0
  │    ├─ record generatedAt = time.Now()
  │    ├─ semaphore <- struct{}{}            // acquire slot (blocks if full)
  │    └─ go executeRequest()
  │
  └─ wait for all request goroutines to finish

executeRequest() (per request)
  │
  ├─ defer func() { <-semaphore }()          // release slot
  ├─ result = doCompletionRequest(...)        // reuse existing function
  ├─ result.QueueTime = sendTime - generatedAt - poissonSleep
  ├─ results <- result
  └─ onProgress(...)
```

### Poisson Inter-Arrival Time

```go
// Exponential distribution: interval = -ln(1 - rand) / rate
interval := time.Duration(-math.Log(1.0-rand.Float64()) / float64(requestRate) * float64(time.Second))
```

Uses `math/rand` with a default source. No seed needed — each run should produce different inter-arrival sequences.

### Semaphore

```go
semaphore := make(chan struct{}, maxInFlight)
```

Buffered channel used as a counting semaphore. Generator acquires before launching each request goroutine; goroutine releases on completion.

### Queue Time

```go
// QueueTime = time from generation to actual send, minus the intentional Poisson sleep
queueTime := sendTime.Sub(generatedAt) - poissonSleep
```

If queue time is negative (timer fired slightly early), clamp to 0.

### Cancellation

- `ctx.Done()` in generator: stop generating new requests, drain remaining semaphore acquisitions
- `ctx.Done()` in `doCompletionRequest`: existing behavior (request fails with context error)
- Waiting on semaphore with ctx: `select { case semaphore <- struct{}{}: case <-ctx.Done(): }` — request is recorded as error, does not occupy a slot

### Progress

Same as closed-loop: each completed request calls `onProgress(ProgressUpdate{Completed, Errors})`. Progress bar percentage = (completed + errors) / TotalRequests.

## Report Changes

### CompletionReport (internal/bench/stats.go)

Four new fields:

```go
// Open-loop queue time metrics (only meaningful when LoadModel == "open_loop")
QueueTimeAvg float64
QueueTimeP50 float64
QueueTimeP90 float64
QueueTimeP99 float64
```

### completionResult (internal/bench/completion.go)

One new field:

```go
QueueTime time.Duration
```

### Display Rules

- **Closed-loop**: QueueTime fields are all zero; results screen hides Queue Time rows
- **Open-loop single**: Queue Time Avg/P50/P90/P99 displayed in a dedicated section
- **Open-loop PK**: Queue Time rows shown for both providers with winner highlighting
- **Mixed PK** (one closed-loop, one open-loop): Not allowed — PK mode enforces same Load Model

## TUI Changes

### Config Screen (internal/tui/config_screen.go)

#### New Field Type

Add `fieldType` to `fieldDef`:

```go
type fieldDef struct {
    label       string
    placeholder string
    defaultVal  string
    password    bool
    fieldType   string // "" (textinput, default) or "toggle"
}
```

#### Load Model Toggle

When `focusIndex` is on the Load Model field:
- `←` / `→` keys toggle between closed-loop and open-loop
- Rendered as radio buttons: `(●) closed-loop  ( ) open-loop`
- Default: closed-loop (index 0)

#### Dynamic Field Visibility

Field list is built dynamically based on `loadModelIndex`:

| Load Model | Fields Shown |
|------------|-------------|
| closed-loop | Concurrency, Total Requests, Input Tokens, ... |
| open-loop | Max In-Flight, Request Rate, Total Requests, Input Tokens, ... |

`Max In-Flight` placeholder: `"equivalent to Concurrency, e.g. 10"`
`Request Rate` placeholder: `"requests per second, e.g. 10"`

#### Config Model State

```go
type configModel struct {
    // ... existing fields ...
    loadModelIndex int // 0 = closed-loop, 1 = open-loop
}
```

The toggle occupies one `focusIndex` slot in the field list. Tab/shift+tab navigates to it like any other field. When focused, `←`/`→` toggles; all other keys pass through to textinput as before (but textinput is not rendered for this field).

#### Validation

- `LoadModel` is set from `loadModelIndex` (no user text input, always valid)
- Open-loop: `MaxInFlight` must be a positive integer
- Open-loop: `RequestRate` must be a positive integer
- Closed-loop: `Concurrency` must be a positive integer (existing behavior)

### Running Screen (internal/tui/running_screen.go)

No changes. Progress reporting works identically for both models.

### Results Screen (internal/tui/results_screen.go)

- When `CompletionReport.QueueTimeAvg > 0` (or any QueueTime field is non-zero), display Queue Time section
- PK mode: Queue Time rows added to the comparison table
- Closed-loop results: Queue Time rows omitted entirely

### startBench (internal/tui/running_screen.go)

```go
if cfg.LoadModel == "open_loop" {
    r := bench.RunOpenLoopCompletionBench(ctx, pv, cfg, testText, actualTokens, tkm, onProgress)
    p.Send(BenchDoneMsg{ProviderIndex: idx, CompletionReport: &r})
} else {
    // existing dispatch logic unchanged
}
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Generator ctx cancelled | Stop generating; in-flight requests complete or are cancelled via ctx |
| HTTP request fails | Record in ErrorDetails/ErrorCategories (same as closed-loop) |
| Semaphore wait + ctx cancel | Request recorded as error; semaphore slot not occupied |
| All requests fail | `Valid = false`; results screen shows "All requests failed" |

## Testing

### Unit Tests — internal/bench/openloop_test.go

| Test | What It Verifies |
|------|-----------------|
| Poisson interval distribution | Statistical mean of generated intervals ≈ 1/rate |
| Semaphore concurrency limit | Max simultaneous in-flight requests ≤ MaxInFlight (mock HTTP server) |
| Queue time calculation | QueueTime ≈ actual wait time, not negative |
| Report aggregation | QueueTimeAvg/P50/P90/P99 computed correctly from results |
| Cancellation | Generator stops, no goroutine leaks |
| Zero total requests | Returns empty report without panic |
| Zero request rate | Treated as invalid (validation catches it) |

### Config Validation Tests — internal/tui/config_screen_test.go

| Test | What It Verifies |
|------|-----------------|
| Open-loop requires MaxInFlight | Error when empty or non-positive |
| Open-loop requires RequestRate | Error when empty or non-positive |
| Closed-loop ignores open-loop fields | No error when MaxInFlight/RequestRate are empty |
| Toggle switches field visibility | Field count changes when loadModelIndex changes |

## File Changes Summary

| File | Change |
|------|--------|
| `internal/bench/config.go` | Add LoadModel, RequestRate, MaxInFlight to BenchConfig |
| `internal/bench/completion.go` | Add QueueTime to completionResult |
| `internal/bench/stats.go` | Add QueueTimeAvg/P50/P90/P99 to CompletionReport |
| `internal/bench/openloop.go` | **New** — RunOpenLoopCompletionBench + scheduler |
| `internal/bench/openloop_test.go` | **New** — unit tests |
| `internal/tui/config_screen.go` | Add fieldType, Load Model toggle, dynamic fields, validation |
| `internal/tui/config_screen_test.go` | **New** — config validation tests |
| `internal/tui/running_screen.go` | Add open-loop dispatch in startBench |
| `internal/tui/results_screen.go` | Add Queue Time display rows |
| `AGENTS.md` | Document open-loop mode in architecture section |
