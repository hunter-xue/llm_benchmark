package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	return doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "", nil)
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
	if !strings.Contains(res.Err.Error(), "only reasoning tokens received") {
		t.Errorf("error = %q, want it to mention %q", res.Err.Error(), "only reasoning tokens received")
	}
}

func TestDoCompletionRequest_ReasoningOnlyStreamToggleOffIsError(t *testing.T) {
	server := reasoningOnlyServer(t)
	defer server.Close()

	// Even when the TTFT toggle ignores reasoning for timing, a reasoning-only
	// stream must still be reported as reasoning-only (not "no output tokens").
	res := doSingleCompletion(t, server, false)
	if res.Err == nil {
		t.Fatal("reasoning-only stream should be an error")
	}
	if !strings.Contains(res.Err.Error(), "only reasoning tokens received") {
		t.Errorf("error = %q, want it to mention %q", res.Err.Error(), "only reasoning tokens received")
	}
}

// emptyStreamServer emits one role-only delta and [DONE] — neither content
// nor reasoning.
func emptyStreamServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func TestDoCompletionRequest_EmptyStreamIsError(t *testing.T) {
	server := emptyStreamServer(t)
	defer server.Close()

	res := doSingleCompletion(t, server, true)
	if res.Err == nil {
		t.Fatal("empty stream should be an error")
	}
	if !strings.Contains(res.Err.Error(), "no output tokens received") {
		t.Errorf("error = %q, want it to mention %q", res.Err.Error(), "no output tokens received")
	}
	if strings.Contains(res.Err.Error(), "only reasoning") {
		t.Errorf("error = %q, should not be the reasoning-only message for an empty stream", res.Err.Error())
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

// reasoningFieldThenContentServer is like reasoningThenContentServer but uses
// the OpenRouter-style `reasoning` delta field instead of `reasoning_content`.
func reasoningFieldThenContentServer(t *testing.T, gap time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking...\"}}]}\n\n")
		flusher.Flush()
		time.Sleep(gap)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func TestDoCompletionRequest_TTFTIncludesReasoningField(t *testing.T) {
	gap := 150 * time.Millisecond
	server := reasoningFieldThenContentServer(t, gap)
	defer server.Close()

	res := doSingleCompletion(t, server, true)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.TTFT >= gap {
		t.Errorf("TTFT=%v should stop at the `reasoning`-field chunk (< %v)", res.TTFT, gap)
	}
}

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

// anthropicThinkingOnlyServer emits one thinking_delta then message_stop,
// without any text_delta.
func anthropicThinkingOnlyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "event: content_block_delta\n")
		fmt.Fprintf(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"hmm\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_stop\n")
		fmt.Fprintf(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
}

func TestDoAnthropicRequest_ThinkingOnlyStreamIsError(t *testing.T) {
	server := anthropicThinkingOnlyServer(t)
	defer server.Close()

	res := doSingleAnthropic(t, server, true)
	if res.Err == nil {
		t.Fatal("thinking-only stream should be an error")
	}
	if !strings.Contains(res.Err.Error(), "only reasoning tokens received") {
		t.Errorf("error = %q, want it to mention %q", res.Err.Error(), "only reasoning tokens received")
	}
}

// TestDoCompletionRequest_WirePayload verifies the actual JSON bytes sent to the
// provider: model/messages/stream, max_tokens when MaxOutputTokens > 0, and
// stream_options.include_usage.
func TestDoCompletionRequest_WirePayload(t *testing.T) {
	var captured []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "kimi-k2.6"}
	cfg := BenchConfig{Mode: ModeCompletion, MaxOutputTokens: 1024, TTFTIncludesReasoning: true}
	client := &http.Client{Timeout: 10 * time.Second}
	res := doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "", nil)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}

	var payload map[string]any
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("captured body is not JSON: %v\nbody: %s", err, captured)
	}
	t.Logf("wire payload: %s", captured)
	if payload["model"] != "kimi-k2.6" {
		t.Errorf("model = %v, want kimi-k2.6", payload["model"])
	}
	if payload["stream"] != true {
		t.Errorf("stream = %v, want true", payload["stream"])
	}
	if payload["max_tokens"] != float64(1024) {
		t.Errorf("max_tokens = %v, want 1024", payload["max_tokens"])
	}
	so, ok := payload["stream_options"].(map[string]any)
	if !ok || so["include_usage"] != true {
		t.Errorf("stream_options.include_usage = %v, want true", payload["stream_options"])
	}
}
