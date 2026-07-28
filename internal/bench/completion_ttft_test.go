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
