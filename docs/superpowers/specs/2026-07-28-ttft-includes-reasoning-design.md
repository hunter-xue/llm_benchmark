# TTFT Includes Reasoning Toggle — Design Spec

## Overview

Add a `TTFT Includes Reasoning` toggle to the Chat Completion and Anthropic Messages benchmark config. When ON (default), TTFT stops at the first reasoning/thinking token; when OFF, TTFT keeps the current behavior (first answer content token). The toggle affects only the TTFT timing point — output token statistics (OutputTokens / TPOT / OutputTPS / TPM) remain content-only and unchanged.

## Background

Reasoning models (DeepSeek R1, QwQ, Kimi-thinking, Claude extended thinking, etc.) stream reasoning tokens before answer tokens:

- OpenAI-compatible APIs emit `delta.reasoning_content` (DeepSeek / vLLM / SGLang / Kimi / NVIDIA NIM) or `delta.reasoning` (OpenRouter and others)
- Anthropic Messages emits `content_block_delta` events with `delta.type == "thinking_delta"`

The current implementation only recognizes answer content (`delta.content` / `text_delta`). For a reasoning model that thinks for 30s before answering, measured TTFT is ~30s+ — actually "time to first answer token", not the strict time-to-first-token. Strictly, the first reasoning token marks the start of generation and should stop the TTFT clock (matching e.g. vLLM benchmark_serving semantics).

## Scope

- **API Modes**: Chat Completion and Anthropic Messages (Embedding mode unaffected)
- **Test Modes**: Single Provider and PK Mode (toggle is a shared parameter in PK)
- **Load Models**: Closed-loop and open-loop both inherit the change (open-loop reuses `doCompletionRequest`)
- **Metric scope**: TTFT only. OutputTokens / TPOT / OutputTPS / TPM / AvgOutputTokens stay content-only
- **Non-streaming paths**: Single Response View, Response Compare, Prompt Cache Hit Test are unaffected

## Config Changes

### BenchConfig (internal/bench/config.go)

One new field:

```go
type BenchConfig struct {
    // ... existing fields unchanged ...

    TTFTIncludesReasoning bool // true: first reasoning/thinking token stops the TTFT clock
}
```

- Default is `true` (set by the TUI config screen; zero value is `false`, matching legacy behavior for any other construction path)

## Bench-Side Measurement Semantics

### Core Principle

Reasoning can only make TTFT stop **earlier**. It can never turn a failed request into a success, and it never enters any output statistic.

### completion.go (OpenAI-compatible mode)

1. `streamChunk.Choices[].Delta` gains two fields:

   ```go
   ReasoningContent string `json:"reasoning_content"` // DeepSeek / vLLM / SGLang / Kimi / NIM
   Reasoning        string `json:"reasoning"`         // OpenRouter and others
   ```

2. SSE loop — success detection and TTFT timing are separated:
   - New variable `gotContent`, set only when `content` is non-empty
   - Success check changes from `!gotFirstToken` to `!gotContent` (error message unchanged: "no output tokens received")
   - Toggle ON: a chunk with non-empty `content`, `reasoning_content`, or `reasoning` refreshes `firstTokenTime` (once) and `lastTokenTime`
   - Toggle OFF: only non-empty `content` refreshes timestamps — code path identical to today
   - Reasoning text is never written to `outputBuf`, never counted in OutputTokens

3. `firstTokenTime`/`lastTokenTime` refresh rule when toggle is ON: reasoning chunks refresh both timestamps (same as content chunks). This is harmless for E2E because in any successful stream the last token is a content token.

### anthropic.go (Anthropic Messages mode)

- Toggle ON: `content_block_delta` events with `delta.type == "thinking_delta"` also stop the TTFT clock (not written to `outputBuf`)
- Success check changes to `gotContent` (set only by non-empty `text_delta`) — behavior identical to today
- The thinking text itself is not parsed; only the delta type drives timing

### Report Structures

No changes. `CompletionReport` and `completionResult` gain no new fields.

## Edge Cases

| Scenario | Behavior (toggle ON) | Notes |
|----------|---------------------|-------|
| Non-reasoning model (no reasoning chunks ever) | First content token stops TTFT; identical to toggle OFF | Falls through to the content check naturally |
| `reasoning_content`/`reasoning` missing, `null`, or empty string | Skipped, does not stop the clock | Non-empty check filters these (e.g. Qwen thinking disabled, DeepSeek non-reasoning mode) |
| Reasoning-only stream (max_tokens exhausted by reasoning, no answer) | Error "no output tokens received"; sample excluded from stats | Same as today; TTFT clock may have stopped but the errored sample is discarded |
| `<think>` tags inline in `content` (some OSS deployments) | Treated as content; first `<think>` chunk stops TTFT under both toggle states | Indistinguishable from answer text; documented limitation |
| Whitespace-only reasoning | Counts (non-empty check), same standard as content | No trimming — stays consistent with existing content handling |
| First chunk carries only `delta.role` | Skipped (all fields empty) | Same as today |

### Known Metric Characteristic

With the toggle ON, `E2E − TTFT` includes the reasoning phase duration while the TPOT denominator counts answer tokens only. This is the accepted behavior of the chosen scope ("toggle affects TTFT only").

## TUI Changes

### Config Screen (internal/tui/config_screen.go)

#### State

```go
type configModel struct {
    // ... existing fields ...
    ttftReasoningOn bool // default true, set in newConfigModel
}
```

Like `loadModelIndex`, this state lives outside the textinput list and survives `rebuildFields()`.

#### Toggle Dispatch Generalization

Currently `←`/`→` hardcodes `toggleLoadModel()`. Generalize to dispatch by the focused field's label:

- `"Load Model"` → existing `toggleLoadModel()` logic
- `"TTFT Includes Reasoning"` → flip `ttftReasoningOn`

Rendering generalizes the same way:

- Load Model: `(●) closed-loop  ( ) open-loop`
- TTFT Includes Reasoning: `(●) yes  ( ) no`

#### Field Placement

- Shown when `isCompletionLike` (Chat Completion + Anthropic Messages), hidden for Embedding
- Position: after "Max Output Tokens", before "System Prompt"
- PK mode: automatically falls into the shared-parameters section (`renderSection`'s `len-10` count unaffected)

#### Validation

`validate()` sets `cfg.TTFTIncludesReasoning = m.ttftReasoningOn` when `isCompletionLike`. No text input, always valid.

#### Help Text

No change — `isToggleField()` already shows `←/→ toggle` for any toggle field.

### Running / Results Screens

No changes. Progress reporting, result tables, and PK comparison are unaffected.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Malformed SSE JSON line | `skippedChunks++` (unchanged) |
| Reasoning-only stream | Error "no output tokens received", counted in ErrorDetails/ErrorCategories |
| HTTP error / stream error | Unchanged |
| All requests fail | `Valid = false` (unchanged) |

## Testing

### Unit Tests — internal/bench/completion_ttft_test.go (new)

All new bench tests live in this single file (both modes share `completionResult` and there is no existing anthropic test file). Use `httptest.Server` with controlled SSE chunk timing (send reasoning chunk, sleep, then content chunk; assert TTFT relative to the sleep):

| Test | What It Verifies |
|------|-----------------|
| reasoning→content stream, toggle ON | TTFT stops at first reasoning chunk (TTFT < chunk gap) |
| reasoning→content stream, toggle OFF | TTFT stops at first content chunk (TTFT ≥ chunk gap) |
| content-only stream, toggle ON | TTFT identical to toggle OFF |
| reasoning-only stream, toggle ON | Error "no output tokens received" |
| OutputTokens with toggle ON | Reasoning text not counted in OutputTokens |
| Anthropic thinking_delta→text_delta, toggle ON | TTFT stops at first thinking delta |

### Config Screen Tests — internal/tui/config_screen_test.go (additions)

| Test | What It Verifies |
|------|-----------------|
| Toggle visible in Chat Completion / Anthropic Messages | Field present in fieldDefs |
| Toggle hidden in Embedding mode | Field absent |
| Default value | `ttftReasoningOn == true`; `validate()` injects `cfg.TTFTIncludesReasoning == true` |
| Toggle dispatch | `←`/`→` on the new toggle flips only its own state, not `loadModelIndex`, and vice versa |

## File Changes Summary

| File | Change |
|------|--------|
| `internal/bench/config.go` | Add `TTFTIncludesReasoning` to BenchConfig |
| `internal/bench/completion.go` | Parse reasoning fields; `gotContent` success check; toggle-aware TTFT timing |
| `internal/bench/anthropic.go` | Recognize `thinking_delta`; `gotContent` success check; toggle-aware TTFT timing |
| `internal/bench/completion_ttft_test.go` | **New** — TTFT/reasoning unit tests |
| `internal/tui/config_screen.go` | Generalize toggle dispatch; add toggle field, state, rendering, validation |
| `internal/tui/config_screen_test.go` | Add toggle tests |
| `AGENTS.md` | Document new BenchConfig field and TTFT semantics |

## Non-Goals (YAGNI)

- No toggle-state annotation on the results screen (the user configured it themselves)
- Reasoning tokens are not counted in any output statistic
- No parsing of inline `<think>` tags in content
- No changes to non-streaming paths (Single Response View, Response Compare, Prompt Cache Hit Test)
