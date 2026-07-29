package bench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var errTest = errors.New("test error")

func TestRedactAuthorization(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Bearer sk-abcdefgh", "Bearer ***efgh"},
		{"sk-abcdefgh", "***efgh"},
		{"Bearer abc", "Bearer ***"},
		{"abcd", "***"},
		{"", ""},
	}
	for _, c := range cases {
		if got := redactAuthorization(c.in); got != c.want {
			t.Errorf("redactAuthorization(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFlattenHeaders_RedactsAuthorization(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Authorization", "Bearer sk-abcdefgh")
	flat := flattenHeaders(h)
	if flat["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", flat["Content-Type"])
	}
	if flat["Authorization"] != "Bearer ***efgh" {
		t.Errorf("Authorization = %q, want redacted with last-4", flat["Authorization"])
	}
	// Original header map must not be mutated.
	if h.Get("Authorization") != "Bearer sk-abcdefgh" {
		t.Error("flattenHeaders must not mutate the source header map")
	}
}

func TestRequestLogger_EnqueueDropsWhenFull(t *testing.T) {
	l := &RequestLogger{ch: make(chan logEntry, 1)}
	l.enqueue(logEntry{req: &requestLogEntry{RequestID: "a"}})
	l.enqueue(logEntry{req: &requestLogEntry{RequestID: "b"}}) // channel full -> dropped
	if got := l.DroppedCount(); got != 1 {
		t.Errorf("DroppedCount = %d, want 1", got)
	}
}

func TestRequestLogger_WritesJSONL(t *testing.T) {
	dir := t.TempDir()
	l, err := NewRequestLogger(dir, "20260729-153045")
	if err != nil {
		t.Fatalf("NewRequestLogger: %v", err)
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Authorization", "Bearer sk-abcdefgh")
	l.LogRequest("20260729-153045-000001", "POST", "http://example.com/v1/chat/completions", h, []byte(`{"model":"m","stream":true}`))
	l.LogResponse("20260729-153045-000001", "200 OK", http.Header{"Content-Type": []string{"text/event-stream"}}, []string{"data: {\"a\":1}", "data: [DONE]"}, "", nil, 842*time.Millisecond)
	l.Close()

	reqBytes, err := os.ReadFile(filepath.Join(dir, "bench_requests_20260729-153045.jsonl"))
	if err != nil {
		t.Fatalf("read requests file: %v", err)
	}
	var reqEntry requestLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(reqBytes))), &reqEntry); err != nil {
		t.Fatalf("requests line is not valid JSON: %v", err)
	}
	if reqEntry.RequestID != "20260729-153045-000001" {
		t.Errorf("request_id = %q", reqEntry.RequestID)
	}
	if reqEntry.TS == "" {
		t.Error("ts must not be empty")
	}
	if reqEntry.Method != "POST" || reqEntry.URL != "http://example.com/v1/chat/completions" {
		t.Errorf("method/url = %q %q", reqEntry.Method, reqEntry.URL)
	}
	if reqEntry.Headers["Authorization"] != "Bearer ***efgh" {
		t.Errorf("logged Authorization = %q, want redacted", reqEntry.Headers["Authorization"])
	}
	if string(reqEntry.Body) != `{"model":"m","stream":true}` {
		t.Errorf("body = %s, want exact wire bytes", reqEntry.Body)
	}
	var reqMap map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(reqBytes))), &reqMap); err != nil {
		t.Fatalf("requests line map unmarshal: %v", err)
	}
	for _, k := range []string{"request_id", "ts", "method", "url", "headers", "body"} {
		if _, ok := reqMap[k]; !ok {
			t.Errorf("requests line missing key %q (keys: %v)", k, reqMap)
		}
	}

	respBytes, err := os.ReadFile(filepath.Join(dir, "bench_responses_20260729-153045.jsonl"))
	if err != nil {
		t.Fatalf("read responses file: %v", err)
	}
	var respEntry responseLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(respBytes))), &respEntry); err != nil {
		t.Fatalf("responses line is not valid JSON: %v", err)
	}
	if respEntry.RequestID != "20260729-153045-000001" {
		t.Errorf("request_id = %q", respEntry.RequestID)
	}
	if respEntry.Status != "200 OK" {
		t.Errorf("status = %q", respEntry.Status)
	}
	if respEntry.DurationMs != 842 {
		t.Errorf("duration_ms = %d, want 842", respEntry.DurationMs)
	}
	if len(respEntry.Chunks) != 2 || respEntry.Chunks[0] != "data: {\"a\":1}" || respEntry.Chunks[1] != "data: [DONE]" {
		t.Errorf("chunks = %v", respEntry.Chunks)
	}
	if respEntry.Error != "" {
		t.Errorf("error = %q, want empty", respEntry.Error)
	}
	var respMap map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(respBytes))), &respMap); err != nil {
		t.Fatalf("responses line map unmarshal: %v", err)
	}
	for _, k := range []string{"request_id", "ts", "duration_ms", "status", "headers", "chunks"} {
		if _, ok := respMap[k]; !ok {
			t.Errorf("responses line missing key %q (keys: %v)", k, respMap)
		}
	}
	if _, ok := respMap["error"]; ok {
		t.Errorf("responses line must omit %q for the success case, got %v", "error", respMap["error"])
	}
	if got := l.DroppedCount(); got != 0 {
		t.Errorf("DroppedCount = %d, want 0", got)
	}
}

func TestRequestLogger_ResponseEntryWithError(t *testing.T) {
	dir := t.TempDir()
	l, err := NewRequestLogger(dir, "run1")
	if err != nil {
		t.Fatalf("NewRequestLogger: %v", err)
	}
	l.LogResponse("run1-000001", "500 Internal Server Error", http.Header{}, nil, `{"error":"boom"}`, errTest, 5*time.Millisecond)
	l.Close()

	respBytes, err := os.ReadFile(filepath.Join(dir, "bench_responses_run1.jsonl"))
	if err != nil {
		t.Fatalf("read responses file: %v", err)
	}
	var e responseLogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(respBytes))), &e); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if e.Error != "test error" {
		t.Errorf("error = %q, want %q", e.Error, "test error")
	}
	if e.Body != `{"error":"boom"}` {
		t.Errorf("body = %q", e.Body)
	}
	if e.Status != "500 Internal Server Error" {
		t.Errorf("status = %q", e.Status)
	}
	if e.Chunks != nil {
		t.Errorf("chunks = %v, want nil", e.Chunks)
	}
}

// loggedRequestServer streams two content chunks and [DONE].
func loggedRequestServer(t *testing.T) *httptest.Server {
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

func readJSONLLines(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func TestDoCompletionRequest_LogsRequestAndResponse(t *testing.T) {
	server := loggedRequestServer(t)
	defer server.Close()

	dir := t.TempDir()
	logger, err := NewRequestLogger(dir, "run1")
	if err != nil {
		t.Fatalf("NewRequestLogger: %v", err)
	}

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model", APIKey: "sk-abcdefgh"}
	cfg := BenchConfig{Mode: ModeCompletion, TTFTIncludesReasoning: true, RequestLogging: true}
	client := &http.Client{Timeout: 10 * time.Second}
	res := doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "run1-000001", logger)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	logger.Close()

	reqLines := readJSONLLines(t, filepath.Join(dir, "bench_requests_run1.jsonl"))
	if len(reqLines) != 1 {
		t.Fatalf("requests lines = %d, want 1", len(reqLines))
	}
	var reqEntry requestLogEntry
	if err := json.Unmarshal([]byte(reqLines[0]), &reqEntry); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	if reqEntry.RequestID != "run1-000001" {
		t.Errorf("request_id = %q", reqEntry.RequestID)
	}
	if reqEntry.URL != server.URL {
		t.Errorf("url = %q, want %q", reqEntry.URL, server.URL)
	}
	if reqEntry.Headers["Authorization"] != "Bearer ***efgh" {
		t.Errorf("Authorization = %q, want redacted", reqEntry.Headers["Authorization"])
	}
	if !strings.Contains(string(reqEntry.Body), `"model":"test-model"`) {
		t.Errorf("body missing model: %s", reqEntry.Body)
	}

	respLines := readJSONLLines(t, filepath.Join(dir, "bench_responses_run1.jsonl"))
	if len(respLines) != 1 {
		t.Fatalf("responses lines = %d, want 1", len(respLines))
	}
	var respEntry responseLogEntry
	if err := json.Unmarshal([]byte(respLines[0]), &respEntry); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if respEntry.RequestID != "run1-000001" {
		t.Errorf("request_id = %q", respEntry.RequestID)
	}
	if respEntry.Status != "200 OK" {
		t.Errorf("status = %q", respEntry.Status)
	}
	// Every raw SSE line (incl. data: prefix and [DONE]) must be logged.
	wantChunks := []string{
		`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
		`data: {"choices":[{"delta":{"content":" world"}}]}`,
		`data: [DONE]`,
	}
	if len(respEntry.Chunks) != len(wantChunks) {
		t.Fatalf("chunks = %v, want %v", respEntry.Chunks, wantChunks)
	}
	for i, want := range wantChunks {
		if respEntry.Chunks[i] != want {
			t.Errorf("chunks[%d] = %q, want %q", i, respEntry.Chunks[i], want)
		}
	}
	if respEntry.Error != "" {
		t.Errorf("error = %q, want empty", respEntry.Error)
	}
}

func TestDoCompletionRequest_LogsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error":"upstream down"}`)
	}))
	defer server.Close()

	dir := t.TempDir()
	logger, err := NewRequestLogger(dir, "run1")
	if err != nil {
		t.Fatalf("NewRequestLogger: %v", err)
	}

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{Mode: ModeCompletion, RequestLogging: true}
	client := &http.Client{Timeout: 10 * time.Second}
	res := doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "run1-000007", logger)
	if res.Err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	logger.Close()

	respLines := readJSONLLines(t, filepath.Join(dir, "bench_responses_run1.jsonl"))
	if len(respLines) != 1 {
		t.Fatalf("responses lines = %d, want 1", len(respLines))
	}
	var e responseLogEntry
	if err := json.Unmarshal([]byte(respLines[0]), &e); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if e.RequestID != "run1-000007" {
		t.Errorf("request_id = %q", e.RequestID)
	}
	if !strings.HasPrefix(e.Status, "500") {
		t.Errorf("status = %q, want 500 ...", e.Status)
	}
	if !strings.Contains(e.Body, "upstream down") {
		t.Errorf("body = %q, want error snippet", e.Body)
	}
	if !strings.Contains(e.Error, "HTTP 500") {
		t.Errorf("error = %q, want HTTP 500 ...", e.Error)
	}
	if len(e.Chunks) != 0 {
		t.Errorf("chunks = %v, want empty", e.Chunks)
	}
}

func TestDoCompletionRequest_NilLoggerWritesNothing(t *testing.T) {
	server := loggedRequestServer(t)
	defer server.Close()

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{Mode: ModeCompletion, TTFTIncludesReasoning: true}
	client := &http.Client{Timeout: 10 * time.Second}
	res := doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "", nil)
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
}
