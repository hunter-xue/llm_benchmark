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
	"regexp"
	"strings"
	"testing"
	"time"
)

var errTest = errors.New("test error")

// requestIDPattern pins the <runID>-<seq> request ID format: YYYYMMDD-HHMMSS-NNNNNN.
var requestIDPattern = regexp.MustCompile(`^\d{8}-\d{6}-\d{6}$`)

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

func TestDoCompletionRequest_NilLoggerNoPanic(t *testing.T) {
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

func TestDoCompletionRequest_LogsTransportError(t *testing.T) {
	// Server closed before the request -> client.Do fails.
	server := loggedRequestServer(t)
	url := server.URL
	server.Close()

	dir := t.TempDir()
	logger, err := NewRequestLogger(dir, "run1")
	if err != nil {
		t.Fatalf("NewRequestLogger: %v", err)
	}

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: url, Model: "test-model"}
	cfg := BenchConfig{Mode: ModeCompletion, RequestLogging: true}
	client := &http.Client{Timeout: 2 * time.Second}
	res := doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "run1-000009", logger)
	if res.Err == nil {
		t.Fatal("expected transport error")
	}
	logger.Close()

	// The request itself must still be logged (it was attempted).
	reqLines := readJSONLLines(t, filepath.Join(dir, "bench_requests_run1.jsonl"))
	if len(reqLines) != 1 {
		t.Fatalf("requests lines = %d, want 1", len(reqLines))
	}

	respLines := readJSONLLines(t, filepath.Join(dir, "bench_responses_run1.jsonl"))
	if len(respLines) != 1 {
		t.Fatalf("responses lines = %d, want 1", len(respLines))
	}
	var e responseLogEntry
	if err := json.Unmarshal([]byte(respLines[0]), &e); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if e.RequestID != "run1-000009" {
		t.Errorf("request_id = %q", e.RequestID)
	}
	if e.Status != "" {
		t.Errorf("status = %q, want empty (no response received)", e.Status)
	}
	if e.Error == "" {
		t.Error("error must be set for transport failure")
	}
	if len(e.Chunks) != 0 {
		t.Errorf("chunks = %v, want empty", e.Chunks)
	}
}

func TestDoCompletionRequest_LogsMidStreamError(t *testing.T) {
	// Server streams one chunk then hijacks and kills the connection.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("server does not support hijacking")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		conn.Close()
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
	client := &http.Client{Timeout: 5 * time.Second}
	res := doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "run1-000010", logger)
	if res.Err == nil {
		t.Fatal("expected stream error")
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
	if e.Status != "200 OK" {
		t.Errorf("status = %q, want 200 OK", e.Status)
	}
	// The partial chunk received before the connection died must be preserved.
	if len(e.Chunks) != 1 || e.Chunks[0] != `data: {"choices":[{"delta":{"content":"Hello"}}]}` {
		t.Errorf("chunks = %v, want the one partial chunk", e.Chunks)
	}
	if e.Error == "" {
		t.Error("error must be set for mid-stream failure")
	}
}

func TestDoCompletionRequest_LogsNoContentStream(t *testing.T) {
	// 200 stream with [DONE] but no content -> "no output tokens received" error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
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
	client := &http.Client{Timeout: 5 * time.Second}
	res := doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "run1-000011", logger)
	if res.Err == nil {
		t.Fatal("expected no-content error")
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
	if !strings.Contains(e.Error, "no output tokens received") {
		t.Errorf("error = %q, want 'no output tokens received'", e.Error)
	}
	if len(e.Chunks) != 1 || e.Chunks[0] != "data: [DONE]" {
		t.Errorf("chunks = %v, want just [DONE]", e.Chunks)
	}
}

func TestRunCompletionBench_LogsEndToEnd(t *testing.T) {
	server := loggedRequestServer(t)
	defer server.Close()

	old := requestLogDir
	requestLogDir = t.TempDir()
	defer func() { requestLogDir = old }()

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model", APIKey: "sk-abcdefgh"}
	cfg := BenchConfig{
		Mode:                  ModeCompletion,
		Concurrency:           2,
		TotalRequests:         3,
		TTFTIncludesReasoning: true,
		RequestLogging:        true,
	}
	report := RunCompletionBench(context.Background(), provider, cfg, "test prompt", 2, tkm, nil)
	if !report.Valid {
		t.Fatalf("report invalid; errors: %v", report.ErrorDetails)
	}
	if report.SuccessCount != 3 {
		t.Errorf("SuccessCount = %d, want 3", report.SuccessCount)
	}
	if report.LogRequestsFile == "" || report.LogResponsesFile == "" {
		t.Fatalf("log file paths empty: %q / %q", report.LogRequestsFile, report.LogResponsesFile)
	}
	if report.LogDroppedCount != 0 {
		t.Errorf("LogDroppedCount = %d, want 0", report.LogDroppedCount)
	}
	if report.LogError != "" {
		t.Errorf("LogError = %q, want empty", report.LogError)
	}

	reqLines := readJSONLLines(t, report.LogRequestsFile)
	respLines := readJSONLLines(t, report.LogResponsesFile)
	if len(reqLines) != 3 || len(respLines) != 3 {
		t.Fatalf("lines = %d requests / %d responses, want 3/3", len(reqLines), len(respLines))
	}

	// Every request ID has exactly one request entry and one response entry.
	reqIDs := make(map[string]bool)
	for _, line := range reqLines {
		var e requestLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("invalid request JSON: %v", err)
		}
		if !requestIDPattern.MatchString(e.RequestID) {
			t.Errorf("request_id %q does not match format YYYYMMDD-HHMMSS-NNNNNN", e.RequestID)
		}
		if e.Headers["Authorization"] != "Bearer ***efgh" {
			t.Errorf("Authorization = %q, want redacted", e.Headers["Authorization"])
		}
		reqIDs[e.RequestID] = false
	}
	if len(reqIDs) != 3 {
		t.Fatalf("distinct request IDs = %d, want 3", len(reqIDs))
	}
	for _, line := range respLines {
		var e responseLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("invalid response JSON: %v", err)
		}
		if _, ok := reqIDs[e.RequestID]; !ok {
			t.Errorf("response request_id %q has no request entry", e.RequestID)
		}
		reqIDs[e.RequestID] = true
		if len(e.Chunks) == 0 {
			t.Errorf("response %q has no chunks", e.RequestID)
		}
	}
	for id, matched := range reqIDs {
		if !matched {
			t.Errorf("request %q has no response entry", id)
		}
	}
}

func TestRunCompletionBench_LoggingDisabledByDefault(t *testing.T) {
	server := loggedRequestServer(t)
	defer server.Close()

	dir := t.TempDir()
	old := requestLogDir
	requestLogDir = dir
	defer func() { requestLogDir = old }()

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{Mode: ModeCompletion, Concurrency: 1, TotalRequests: 1, TTFTIncludesReasoning: true}
	report := RunCompletionBench(context.Background(), provider, cfg, "test prompt", 2, tkm, nil)
	if !report.Valid {
		t.Fatalf("report invalid; errors: %v", report.ErrorDetails)
	}
	if report.LogRequestsFile != "" || report.LogResponsesFile != "" {
		t.Errorf("log paths should be empty when disabled: %q / %q", report.LogRequestsFile, report.LogResponsesFile)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("no log files should be created, found %d", len(entries))
	}
}

func TestRunOpenLoopCompletionBench_LogsEndToEnd(t *testing.T) {
	server := loggedRequestServer(t)
	defer server.Close()

	old := requestLogDir
	requestLogDir = t.TempDir()
	defer func() { requestLogDir = old }()

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:                  ModeCompletion,
		LoadModel:             LoadModelOpenLoop,
		RequestRate:           100,
		MaxInFlight:           2,
		Concurrency:           2,
		TotalRequests:         3,
		TTFTIncludesReasoning: true,
		RequestLogging:        true,
	}
	report := RunOpenLoopCompletionBench(context.Background(), provider, cfg, "test prompt", 2, tkm, nil)
	if !report.Valid {
		t.Fatalf("report invalid; errors: %v", report.ErrorDetails)
	}
	if report.SuccessCount != 3 {
		t.Errorf("SuccessCount = %d, want 3", report.SuccessCount)
	}

	reqLines := readJSONLLines(t, report.LogRequestsFile)
	respLines := readJSONLLines(t, report.LogResponsesFile)
	if len(reqLines) != 3 || len(respLines) != 3 {
		t.Fatalf("lines = %d requests / %d responses, want 3/3", len(reqLines), len(respLines))
	}
	ids := make(map[string]int)
	for _, line := range respLines {
		var e responseLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		ids[e.RequestID]++
	}
	if len(ids) != 3 {
		t.Errorf("distinct response request IDs = %d, want 3", len(ids))
	}
}

func TestRunCompletionBench_LoggingWithCancellation(t *testing.T) {
	// Server that streams one chunk then hangs, so cancellation hits mid-run.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()
		<-r.Context().Done() // hang until client cancels
	}))
	defer server.Close()

	old := requestLogDir
	requestLogDir = t.TempDir()
	defer func() { requestLogDir = old }()

	ctx, cancel := context.WithCancel(context.Background())
	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:                  ModeCompletion,
		Concurrency:           2,
		TotalRequests:         5,
		TTFTIncludesReasoning: true,
		RequestLogging:        true,
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	report := RunCompletionBench(ctx, provider, cfg, "test prompt", 2, tkm, nil)

	// Must not hang, must close the logger cleanly, and every logged request
	// must have exactly one logged response (1:1 correlation even on cancel).
	reqLines := readJSONLLines(t, report.LogRequestsFile)
	respLines := readJSONLLines(t, report.LogResponsesFile)
	if len(reqLines) == 0 {
		t.Fatal("expected at least one logged request")
	}
	if len(reqLines) != len(respLines) {
		t.Fatalf("request/response lines = %d/%d, want equal", len(reqLines), len(respLines))
	}
	respIDs := make(map[string]bool)
	for _, line := range respLines {
		var e responseLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("invalid response JSON: %v", err)
		}
		respIDs[e.RequestID] = true
	}
	for _, line := range reqLines {
		var e requestLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("invalid request JSON: %v", err)
		}
		if !respIDs[e.RequestID] {
			t.Errorf("request %q has no response entry", e.RequestID)
		}
	}
}
