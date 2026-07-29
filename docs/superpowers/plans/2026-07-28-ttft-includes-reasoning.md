# TTFT Includes Reasoning Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `TTFT Includes Reasoning` toggle (default ON) so TTFT stops at the first reasoning/thinking token for Chat Completion and Anthropic Messages benchmarks, while output token statistics remain content-only.

**Architecture:** `BenchConfig` gains `TTFTIncludesReasoning bool`. The OpenAI-compatible SSE parser (`completion.go`) recognizes `delta.reasoning_content`/`delta.reasoning`; the Anthropic SSE parser (`anthropic.go`) recognizes `thinking_delta`. Reasoning chunks only stop the TTFT clock — a new `gotContent` flag keeps success semantics tied to answer content. The TUI config screen generalizes its hardcoded Load Model toggle into label-dispatched toggles and adds the new toggle for completion-like modes.

**Tech Stack:** Go, bubbletea TUI, `httptest.Server` SSE mocks for tests.

**Spec:** `docs/superpowers/specs/2026-07-28-ttft-includes-reasoning-design.md`

---

### Task 1: completion.go — reasoning-aware TTFT (OpenAI-compatible mode)

**Files:**
- Modify: `internal/bench/config.go` (BenchConfig struct, lines 33-43)
- Modify: `internal/bench/completion.go` (streamChunk lines 42-50; SSE loop lines 299-355)
- Test: `internal/bench/completion_ttft_test.go` (new)

- [ ] **Step 1: Write the failing tests**

Create `internal/bench/completion_ttft_test.go`:

```go
package bench

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// reasoningThenContentServer streams one reasoning_content chunk immediately,
// waits gap, then streams two content chunks and [DONE].
func reasoningThenContentServer(t *testing.T, gap time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking...\"}}]}\n\n")
		flusher.Flush()
		time.Sleep(gap)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// reasoningOnlyServer streams reasoning chunks and [DONE] without any content.
func reasoningOnlyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"thinking...\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

// contentOnlyServer streams two content chunks and [DONE] (no reasoning fields).
func contentOnlyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func doSingleCompletion(t *testing.T, server *httptest.Server, ttftReasoning bool) completionResult {
	t.Helper()
	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:                  ModeCompletion,
		TTFTIncludesReasoning: ttftReasoning,
	}
	client := &http.Client{Timeout: 10 * time.Second}
	return doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm)
}

func TestDoCompletionRequest_TTFTIncludesReasoning(t *testing.T) {
	gap := 150 * time.Millisecond
	server := reasoningThenContentServer(t, gap)
	defer server.Close()

	res := doSingleCompletion(t, server, true)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.TTFT >= gap {
		t.Errorf("TTFT=%v should stop at the reasoning chunk (< %v)", res.TTFT, gap)
	}
}

func TestDoCompletionRequest_TTFTExcludesReasoning(t *testing.T) {
	gap := 150 * time.Millisecond
	server := reasoningThenContentServer(t, gap)
	defer server.Close()

	res := doSingleCompletion(t, server, false)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.TTFT < gap {
		t.Errorf("TTFT=%v should wait for the first content chunk (>= %v)", res.TTFT, gap)
	}
}

func TestDoCompletionRequest_ContentOnlyStreamToggleOn(t *testing.T) {
	server := contentOnlyServer(t)
	defer server.Close()

	res := doSingleCompletion(t, server, true)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	want := len(testTokenizer(t).Encode("Hello world", nil, nil))
	if res.OutputTokens != want {
		t.Errorf("OutputTokens=%d, want %d", res.OutputTokens, want)
	}
}

func TestDoCompletionRequest_ReasoningOnlyStreamIsError(t *testing.T) {
	server := reasoningOnlyServer(t)
	defer server.Close()

	res := doSingleCompletion(t, server, true)
	if res.Err == nil {
		t.Fatal("reasoning-only stream should be an error")
	}
	if !strings.Contains(res.Err.Error(), "no output tokens received") {
		t.Errorf("error = %q, want it to mention %q", res.Err.Error(), "no output tokens received")
	}
}

func TestDoCompletionRequest_ReasoningNotCountedInOutput(t *testing.T) {
	server := reasoningThenContentServer(t, 0)
	defer server.Close()

	res := doSingleCompletion(t, server, true)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	want := len(testTokenizer(t).Encode("Hello world", nil, nil))
	if res.OutputTokens != want {
		t.Errorf("OutputTokens=%d, want %d (reasoning text must not be counted)", res.OutputTokens, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/bench/ -run TestDoCompletionRequest -v`
Expected: FAIL to compile — `unknown field 'TTFTIncludesReasoning' in struct literal of type BenchConfig`

- [ ] **Step 3: Implement**

**3a. `internal/bench/config.go`** — add the field to `BenchConfig`:

```go
// BenchConfig holds shared benchmark parameters.
type BenchConfig struct {
	Mode                  string // "embedding" or "completion"
	Concurrency           int
	TotalRequests         int
	TargetTokens          int
	MaxOutputTokens       int    // completion only, 0 = unlimited
	SystemPrompt          string // completion only
	LoadModel             string // "closed_loop" (default) or "open_loop"
	RequestRate           int    // open-loop: requests per second (Poisson rate)
	MaxInFlight           int    // open-loop: max concurrent in-flight requests
	TTFTIncludesReasoning bool   // completion-like: first reasoning/thinking token stops the TTFT clock
}
```

**3b. `internal/bench/completion.go`** — extend `streamChunk`'s Delta:

```go
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *apiUsage `json:"usage"`
}
```

**3c. `internal/bench/completion.go`** — in `doCompletionRequest`, add `gotContent` to the var block:

```go
	var (
		firstTokenTime time.Time
		lastTokenTime  time.Time
		outputBuf      strings.Builder
		gotFirstToken  bool
		gotContent     bool
		skippedChunks  int
	)
```

**3d. `internal/bench/completion.go`** — replace the choices loop body:

```go
		for _, choice := range chunk.Choices {
			content := choice.Delta.Content
			reasoning := choice.Delta.ReasoningContent
			if reasoning == "" {
				reasoning = choice.Delta.Reasoning
			}
			if content == "" && !(cfg.TTFTIncludesReasoning && reasoning != "") {
				continue
			}
			now := time.Now()
			if !gotFirstToken {
				firstTokenTime = now
				gotFirstToken = true
			}
			lastTokenTime = now
			if content != "" {
				gotContent = true
				outputBuf.WriteString(content)
			}
		}
```

**3e. `internal/bench/completion.go`** — change the success check:

```go
	if !gotContent {
		res.Err = fmt.Errorf("no output tokens received")
		return res
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run TestDoCompletionRequest -v`
Expected: PASS (5 tests)

Then regression-check the whole package:

Run: `go test ./internal/bench/ -v`
Expected: PASS (all existing tests unaffected)

- [ ] **Step 5: Commit**

```bash
git add internal/bench/config.go internal/bench/completion.go internal/bench/completion_ttft_test.go
git commit -m "feat: count reasoning tokens toward TTFT in completion mode"
```

---

### Task 2: anthropic.go — thinking_delta-aware TTFT (Anthropic Messages mode)

**Files:**
- Modify: `internal/bench/anthropic.go` (SSE loop lines 283-340)
- Test: `internal/bench/completion_ttft_test.go` (append)

- [ ] **Step 1: Write the failing tests**

Append to `internal/bench/completion_ttft_test.go`:

```go
// anthropicThinkingThenTextServer emits a thinking_delta immediately, waits
// gap, then a text_delta, then message_stop.
func anthropicThinkingThenTextServer(t *testing.T, gap time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "event: content_block_delta\n")
		fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n")
		flusher.Flush()
		time.Sleep(gap)
		fmt.Fprintf(w, "event: content_block_delta\n")
		fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_stop\n")
		fmt.Fprintf(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
}

func doSingleAnthropic(t *testing.T, server *httptest.Server, ttftReasoning bool) completionResult {
	t.Helper()
	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:                  ModeAnthropicMessages,
		TTFTIncludesReasoning: ttftReasoning,
	}
	client := &http.Client{Timeout: 10 * time.Second}
	return doAnthropicRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm)
}

func TestDoAnthropicRequest_TTFTIncludesThinking(t *testing.T) {
	gap := 150 * time.Millisecond
	server := anthropicThinkingThenTextServer(t, gap)
	defer server.Close()

	res := doSingleAnthropic(t, server, true)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.TTFT >= gap {
		t.Errorf("TTFT=%v should stop at the thinking delta (< %v)", res.TTFT, gap)
	}
	want := len(testTokenizer(t).Encode("Hello", nil, nil))
	if res.OutputTokens != want {
		t.Errorf("OutputTokens=%d, want %d (thinking text must not be counted)", res.OutputTokens, want)
	}
}

func TestDoAnthropicRequest_TTFTExcludesThinking(t *testing.T) {
	gap := 150 * time.Millisecond
	server := anthropicThinkingThenTextServer(t, gap)
	defer server.Close()

	res := doSingleAnthropic(t, server, false)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.TTFT < gap {
		t.Errorf("TTFT=%v should wait for the first text delta (>= %v)", res.TTFT, gap)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/bench/ -run TestDoAnthropicRequest -v`
Expected: FAIL — `TestDoAnthropicRequest_TTFTIncludesThinking` fails because TTFT >= gap (thinking deltas are currently ignored)

- [ ] **Step 3: Implement**

**3a. `internal/bench/anthropic.go`** — in `doAnthropicRequest`, add `gotContent` to the var block:

```go
	var (
		firstTokenTime time.Time
		lastTokenTime  time.Time
		outputBuf      strings.Builder
		gotFirstToken  bool
		gotContent     bool
		skippedChunks  int
		eventType      string
	)
```

**3b. `internal/bench/anthropic.go`** — replace the content_block_delta handling:

```go
		if eventType != "content_block_delta" {
			continue
		}
		isText := chunk.Delta.Type == "text_delta" && chunk.Delta.Text != ""
		isThinking := chunk.Delta.Type == "thinking_delta"
		if !isText && !(cfg.TTFTIncludesReasoning && isThinking) {
			continue
		}
		now := time.Now()
		if !gotFirstToken {
			firstTokenTime = now
			gotFirstToken = true
		}
		lastTokenTime = now
		if isText {
			gotContent = true
			outputBuf.WriteString(chunk.Delta.Text)
		}
```

**3c. `internal/bench/anthropic.go`** — change the success check:

```go
	if !gotContent {
		res.Err = fmt.Errorf("no output tokens received")
		return res
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -v`
Expected: PASS (all tests, including Task 1's)

- [ ] **Step 5: Commit**

```bash
git add internal/bench/anthropic.go internal/bench/completion_ttft_test.go
git commit -m "feat: count thinking deltas toward TTFT in anthropic mode"
```

---

### Task 3: TUI config screen — TTFT Includes Reasoning toggle

**Files:**
- Modify: `internal/tui/config_screen.go`
- Test: `internal/tui/config_screen_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/config_screen_test.go`:

```go
func TestBuildFieldDefs_TTFTReasoningToggleVisibility(t *testing.T) {
	completionDefs := buildFieldDefs(bench.ModeCompletion, "single", 0)
	found := false
	for _, d := range completionDefs {
		if d.label == "TTFT Includes Reasoning" {
			found = true
			if d.fieldType != "toggle" {
				t.Error("TTFT Includes Reasoning should have fieldType \"toggle\"")
			}
		}
	}
	if !found {
		t.Error("completion mode should include TTFT Includes Reasoning toggle")
	}

	anthropicDefs := buildFieldDefs(bench.ModeAnthropicMessages, "single", 0)
	if !containsLabel(fieldLabels(anthropicDefs), "TTFT Includes Reasoning") {
		t.Error("anthropic mode should include TTFT Includes Reasoning toggle")
	}

	embeddingDefs := buildFieldDefs(bench.ModeEmbedding, "single", 0)
	if containsLabel(fieldLabels(embeddingDefs), "TTFT Includes Reasoning") {
		t.Error("embedding mode should not include TTFT Includes Reasoning toggle")
	}
}

func TestValidate_TTFTIncludesReasoning(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	if !m.ttftReasoningOn {
		t.Error("expected ttftReasoningOn default true")
	}

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Concurrency":    "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.TTFTIncludesReasoning {
		t.Error("expected cfg.TTFTIncludesReasoning=true by default")
	}

	m.ttftReasoningOn = false
	_, cfg, err = m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TTFTIncludesReasoning {
		t.Error("expected cfg.TTFTIncludesReasoning=false after toggle off")
	}
}

func TestToggleFocusedField_Dispatch(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")

	focusByLabel := func(label string) {
		t.Helper()
		for i, fd := range m.fieldDefs {
			if fd.label == label {
				m.focusIndex = i
				return
			}
		}
		t.Fatalf("field %q not found", label)
	}

	focusByLabel("TTFT Includes Reasoning")
	m.toggleFocusedField()
	if m.ttftReasoningOn {
		t.Error("expected ttftReasoningOn=false after toggle")
	}
	if m.loadModelIndex != 0 {
		t.Error("TTFT toggle should not affect loadModelIndex")
	}

	focusByLabel("Load Model")
	m.toggleFocusedField()
	if m.loadModelIndex != 1 {
		t.Error("expected loadModelIndex=1 after Load Model toggle")
	}
	if m.ttftReasoningOn {
		t.Error("Load Model toggle should not affect ttftReasoningOn")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'TTFT|ToggleFocusedField' -v`
Expected: FAIL to compile — `m.ttftReasoningOn undefined` / `m.toggleFocusedField undefined`

- [ ] **Step 3: Implement**

**3a. `internal/tui/config_screen.go`** — add state to `configModel` and default it in `newConfigModel`:

```go
type configModel struct {
	apiMode         string
	testMode        string
	inputs          []textinput.Model
	fieldDefs       []fieldDef
	focusIndex      int
	err             string
	width           int
	loadModelIndex  int  // 0 = closed-loop, 1 = open-loop (completion only)
	ttftReasoningOn bool // TTFT Includes Reasoning toggle (completion-like modes)
}

func newConfigModel(apiMode, testMode string) configModel {
	m := configModel{
		apiMode:         apiMode,
		testMode:        testMode,
		ttftReasoningOn: true,
	}
	m.rebuildFields()
	if len(m.inputs) > 0 {
		m.inputs[0].Focus()
	}
	return m
}
```

**3b. `internal/tui/config_screen.go`** — in `buildFieldDefs`, define the field next to `loadModelField`:

```go
	// TTFT reasoning toggle field (completion-like modes)
	ttftReasoningField := fieldDef{
		label:     "TTFT Includes Reasoning",
		fieldType: "toggle",
	}
```

**3c. `internal/tui/config_screen.go`** — in `buildFieldDefs`, update BOTH `isCompletionLike` blocks (PK path and single-provider path) to insert the toggle between Max Output Tokens and System Prompt:

```go
	if isCompletionLike {
		defs = append(defs,
			fieldDef{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
			ttftReasoningField,
			fieldDef{label: "System Prompt", placeholder: "optional, leave blank to skip"},
		)
	}
```

**3d. `internal/tui/config_screen.go`** — generalize the `left`/`right` dispatch in `update()`:

```go
		case "left", "right":
			if m.isToggleField() {
				m.toggleFocusedField()
				return m, nil
			}
```

Add the new method (keep `toggleLoadModel` unchanged):

```go
// toggleFocusedField flips the state of the currently focused toggle field.
func (m *configModel) toggleFocusedField() {
	switch m.fieldDefs[m.focusIndex].label {
	case "Load Model":
		m.toggleLoadModel()
	case "TTFT Includes Reasoning":
		m.ttftReasoningOn = !m.ttftReasoningOn
	}
}
```

**3e. `internal/tui/config_screen.go`** — in `view()`, replace the toggle rendering call:

```go
			if m.fieldDefs[i].fieldType == "toggle" {
				toggle := m.renderToggle(m.fieldDefs[i].label)
				sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, toggle))
			} else {
```

Replace `renderLoadModelToggle` with a generalized method:

```go
// renderToggle renders the radio buttons for the toggle field with the given label.
func (m configModel) renderToggle(label string) string {
	switch label {
	case "Load Model":
		if m.loadModelIndex == 0 {
			return "(●) closed-loop  ( ) open-loop"
		}
		return "( ) closed-loop  (●) open-loop"
	case "TTFT Includes Reasoning":
		if m.ttftReasoningOn {
			return "(●) yes  ( ) no"
		}
		return "( ) yes  (●) no"
	}
	return ""
}
```

(Delete the old `renderLoadModelToggle` function — `view()` was its only caller.)

**3f. `internal/tui/config_screen.go`** — in `validate()`, update BOTH `isCompletionLike` blocks (PK path ~line 365, single path ~line 440):

```go
	if isCompletionLike {
		maxTok, _ := strconv.Atoi(vals[fieldIdx("Max Output Tokens")])
		cfg.MaxOutputTokens = maxTok
		cfg.SystemPrompt = vals[fieldIdx("System Prompt")]
		cfg.TTFTIncludesReasoning = m.ttftReasoningOn
	}
```

**3g. `internal/tui/config_screen_test.go`** — update the two existing order assertions that now include the new field:

In `TestBuildFieldDefs_CompletionClosedLoop`:

```go
	assertLabelOrder(t, defs, []string{
		"API URL", "API Key", "Model", "Custom Params",
		"Load Model", "Concurrency", "Total Requests", "Input Tokens",
		"Max Output Tokens", "TTFT Includes Reasoning", "System Prompt",
	})
```

In `TestBuildFieldDefs_CompletionOpenLoop`:

```go
	assertLabelOrder(t, defs, []string{
		"API URL", "API Key", "Model", "Custom Params",
		"Load Model", "Max In-Flight", "Request Rate", "Total Requests", "Input Tokens",
		"Max Output Tokens", "TTFT Includes Reasoning", "System Prompt",
	})
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -v`
Expected: PASS (new tests + updated order assertions + all existing tests)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/config_screen.go internal/tui/config_screen_test.go
git commit -m "feat: add TTFT Includes Reasoning toggle to config screen"
```

---

### Task 4: Documentation + final verification

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Update AGENTS.md**

In the `BenchConfig` code block under "Key Types in `internal/bench`", add the field:

```go
type BenchConfig struct {
    Mode, Concurrency, TotalRequests, TargetTokens, MaxOutputTokens int
    SystemPrompt string
    LoadModel string        // "closed_loop" (default) or "open_loop"
    RequestRate int         // open-loop: requests per second
    MaxInFlight int         // open-loop: max concurrent in-flight
    TTFTIncludesReasoning bool // completion-like: first reasoning/thinking token stops the TTFT clock
}
```

In "Key Design Decisions", add a bullet after the "API usage reporting" bullet:

```markdown
- **TTFT semantics**: For completion-like modes, the `TTFT Includes Reasoning` config toggle (default on) controls whether the first reasoning token stops the TTFT clock — `reasoning_content`/`reasoning` delta fields for OpenAI-compatible APIs, `thinking_delta` events for Anthropic. Request success still requires answer content (reasoning-only streams are errors), and reasoning text never enters OutputTokens/TPOT statistics. Providers that inline `<think>` tags in content are indistinguishable from answer text and always count as content.
```

- [ ] **Step 2: Final verification**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: build succeeds, vet clean, all tests PASS

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: document TTFT reasoning toggle in AGENTS.md"
```
