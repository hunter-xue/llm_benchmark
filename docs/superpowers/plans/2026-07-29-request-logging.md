# Request Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in `Request Logging` toggle to the Chat Completion single-provider benchmark that asynchronously logs every request (headers + exact wire body) and response (status + headers + raw SSE lines) to two JSONL files in the working directory, correlated by a local request ID, without ever delaying measured metrics.

**Architecture:** A new `RequestLogger` in `internal/bench/request_log.go` owns two JSONL files, fed by a buffered channel + single writer goroutine. Request goroutines enqueue raw data via non-blocking sends (drop-on-full with a counter); all serialization and disk IO happen in the writer goroutine. `doCompletionRequest` calls `LogRequest` before `sendTime` and `LogResponse` (via defer) after all timing points are captured, so TTFT/E2E/TPOT measurement paths are untouched. Spec: `docs/superpowers/specs/2026-07-29-request-logging-design.md`.

**Tech Stack:** Go 1.x, bubbletea TUI, httptest for tests. No new dependencies.

---

### Task 1: RequestLogger core (internal/bench/request_log.go)

**Files:**
- Create: `internal/bench/request_log.go`
- Test: `internal/bench/request_log_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/bench/request_log_test.go`:

```go
package bench

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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

	respBytes, _ := os.ReadFile(filepath.Join(dir, "bench_responses_run1.jsonl"))
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
```

`errTest` is a package-level test helper — add at the top of the test file:

```go
var errTest = errors.New("test error")
```

(and add `"errors"` to the test imports.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/bench/ -run 'TestRedactAuthorization|TestFlattenHeaders|TestRequestLogger' -v`
Expected: FAIL — compile error (`undefined: redactAuthorization`, `flattenHeaders`, `RequestLogger`, `NewRequestLogger`, `logEntry`, `requestLogEntry`, `responseLogEntry`).

- [ ] **Step 3: Implement internal/bench/request_log.go**

```go
package bench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// requestLogDir is the directory request/response log files are written to.
// Overridden by tests.
var requestLogDir = "."

const (
	requestLogBufferSize  = 4096
	requestLogFlushEvery  = 64
	requestLogFlushPeriod = 500 * time.Millisecond
)

// requestLogEntry is one JSONL line in the requests file.
type requestLogEntry struct {
	RequestID string            `json:"request_id"`
	TS        string            `json:"ts"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Body      json.RawMessage   `json:"body"`
}

// responseLogEntry is one JSONL line in the responses file.
type responseLogEntry struct {
	RequestID string            `json:"request_id"`
	TS        string            `json:"ts"`
	DurationMs int64            `json:"duration_ms"`
	Status    string            `json:"status,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Chunks    []string          `json:"chunks,omitempty"`
	Body      string            `json:"body,omitempty"` // non-200 error body snippet
	Error     string            `json:"error,omitempty"`
}

// logEntry carries one entry to the writer goroutine; exactly one field is set.
type logEntry struct {
	req  *requestLogEntry
	resp *responseLogEntry
}

// RequestLogger asynchronously writes request/response JSONL files.
// Safe for concurrent use. Enqueue never blocks: entries are dropped
// (and counted) when the writer falls behind.
type RequestLogger struct {
	requestsFile  string
	responsesFile string

	ch      chan logEntry
	wg      sync.WaitGroup
	dropped atomic.Int64

	errMu   sync.Mutex
	firstErr error

	reqFile  *os.File
	respFile *os.File
	reqBuf   *bufio.Writer
	respBuf  *bufio.Writer
}

// NewRequestLogger creates the two JSONL files in dir and starts the writer
// goroutine.
func NewRequestLogger(dir, runID string) (*RequestLogger, error) {
	reqPath := filepath.Join(dir, "bench_requests_"+runID+".jsonl")
	respPath := filepath.Join(dir, "bench_responses_"+runID+".jsonl")
	reqFile, err := os.OpenFile(reqPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", reqPath, err)
	}
	respFile, err := os.OpenFile(respPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		reqFile.Close()
		return nil, fmt.Errorf("open %s: %w", respPath, err)
	}
	l := &RequestLogger{
		requestsFile:  reqPath,
		responsesFile: respPath,
		ch:            make(chan logEntry, requestLogBufferSize),
		reqFile:       reqFile,
		respFile:      respFile,
		reqBuf:        bufio.NewWriter(reqFile),
		respBuf:       bufio.NewWriter(respFile),
	}
	l.wg.Add(1)
	go l.writeLoop()
	return l, nil
}

func (l *RequestLogger) RequestsFile() string  { return l.requestsFile }
func (l *RequestLogger) ResponsesFile() string { return l.responsesFile }
func (l *RequestLogger) DroppedCount() int64   { return l.dropped.Load() }

func (l *RequestLogger) Err() error {
	l.errMu.Lock()
	defer l.errMu.Unlock()
	return l.firstErr
}

// LogRequest enqueues a request entry. headerMap is cloned and Authorization
// redacted synchronously (cheap, happens before the request is sent).
func (l *RequestLogger) LogRequest(reqID, method, url string, headerMap http.Header, body []byte) {
	l.enqueue(logEntry{req: &requestLogEntry{
		RequestID: reqID,
		TS:        logTimestamp(),
		Method:    method,
		URL:       url,
		Headers:   flattenHeaders(headerMap),
		Body:      json.RawMessage(body),
	}})
}

// LogResponse enqueues a response entry. errBody is the non-200 response body
// snippet; reqErr is the request error, if any.
func (l *RequestLogger) LogResponse(reqID, status string, headerMap http.Header, chunks []string, errBody string, reqErr error, dur time.Duration) {
	e := &responseLogEntry{
		RequestID: reqID,
		TS:        logTimestamp(),
		DurationMs: dur.Milliseconds(),
		Status:    status,
		Headers:   flattenHeaders(headerMap),
		Chunks:    chunks,
		Body:      errBody,
	}
	if reqErr != nil {
		e.Error = reqErr.Error()
	}
	l.enqueue(logEntry{resp: e})
}

// enqueue never blocks; drops the entry when the channel is full.
func (l *RequestLogger) enqueue(e logEntry) {
	select {
	case l.ch <- e:
	default:
		l.dropped.Add(1)
	}
}

// Close drains remaining entries, flushes, and closes the files.
func (l *RequestLogger) Close() {
	close(l.ch)
	l.wg.Wait()
	l.reqFile.Close()
	l.respFile.Close()
}

func (l *RequestLogger) writeLoop() {
	defer l.wg.Done()
	ticker := time.NewTicker(requestLogFlushPeriod)
	defer ticker.Stop()
	pending := 0
	flush := func() {
		l.flushBuf()
		pending = 0
	}
	for {
		select {
		case e, ok := <-l.ch:
			if !ok {
				flush()
				return
			}
			l.writeEntry(e)
			pending++
			if pending >= requestLogFlushEvery {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (l *RequestLogger) writeEntry(e logEntry) {
	var (
		line []byte
		err  error
		buf  *bufio.Writer
	)
	if e.req != nil {
		line, err = json.Marshal(e.req)
		buf = l.reqBuf
	} else {
		line, err = json.Marshal(e.resp)
		buf = l.respBuf
	}
	if err != nil {
		l.setErr(err)
		return
	}
	if _, err := buf.Write(line); err != nil {
		l.setErr(err)
		return
	}
	if err := buf.WriteByte('\n'); err != nil {
		l.setErr(err)
	}
}

func (l *RequestLogger) flushBuf() {
	if err := l.reqBuf.Flush(); err != nil {
		l.setErr(err)
	}
	if err := l.respBuf.Flush(); err != nil {
		l.setErr(err)
	}
}

func (l *RequestLogger) setErr(err error) {
	l.errMu.Lock()
	if l.firstErr == nil {
		l.firstErr = err
	}
	l.errMu.Unlock()
}

func logTimestamp() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

// flattenHeaders converts http.Header to a single-valued map and redacts the
// Authorization header. The input map is not modified.
func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	flat := make(map[string]string, len(h))
	for k, vals := range h {
		v := strings.Join(vals, ", ")
		if strings.EqualFold(k, "Authorization") {
			v = redactAuthorization(v)
		}
		flat[k] = v
	}
	return flat
}

// redactAuthorization masks a credential, keeping the last 4 characters when
// the credential part is longer than 4 characters.
func redactAuthorization(v string) string {
	if v == "" {
		return v
	}
	const prefix = "Bearer "
	if rest, ok := strings.CutPrefix(v, prefix); ok {
		return prefix + maskCredential(rest)
	}
	return maskCredential(v)
}

func maskCredential(s string) string {
	if len(s) <= 4 {
		return "***"
	}
	return "***" + s[len(s)-4:]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/bench/ -run 'TestRedactAuthorization|TestFlattenHeaders|TestRequestLogger' -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/bench/request_log.go internal/bench/request_log_test.go
git commit -m "feat: add async RequestLogger with credential redaction"
```

---

### Task 2: BenchConfig.RequestLogging + CompletionReport log fields

**Files:**
- Modify: `internal/bench/config.go:33-44` (BenchConfig struct)
- Modify: `internal/bench/stats.go:27-63` (CompletionReport struct)

- [ ] **Step 1: Add RequestLogging to BenchConfig**

In `internal/bench/config.go`, add one field to `BenchConfig`:

```go
	RequestLogging bool // chat completion single-provider: log request/response to JSONL files
```

placed after `TTFTIncludesReasoning bool`.

- [ ] **Step 2: Add log fields to CompletionReport**

In `internal/bench/stats.go`, add to `CompletionReport` after `MissingAPIUsageCount`:

```go
	LogRequestsFile  string // request logging: path of the requests JSONL file ("" = disabled)
	LogResponsesFile string // request logging: path of the responses JSONL file
	LogDroppedCount  int    // request logging: entries dropped because the disk could not keep up
	LogError         string // request logging: first write error, if any
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/bench/config.go internal/bench/stats.go
git commit -m "feat: add RequestLogging config and report fields"
```

---

### Task 3: Wire logging into doCompletionRequest

This changes the `doCompletionRequest` signature, so all call sites and existing tests are updated in the same task to keep the build green: `completion.go` (runner passes `"", nil` for now), `openloop.go` (same), `completion_ttft_test.go` (2 call sites pass `"", nil`).

**Files:**
- Modify: `internal/bench/completion.go:121` (call site), `:238-377` (doCompletionRequest)
- Modify: `internal/bench/openloop.go:108` (call site)
- Modify: `internal/bench/completion_ttft_test.go:74,294` (call sites)
- Test: `internal/bench/request_log_test.go` (add wiring tests)

- [ ] **Step 1: Write the failing wiring test**

Append to `internal/bench/request_log_test.go`:

```go
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
```

Add `"context"`, `"fmt"`, `"net/http/httptest"` to the test file imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run 'TestDoCompletionRequest_Logs' -v`
Expected: FAIL — compile error: "too many arguments in call to doCompletionRequest".

- [ ] **Step 3: Change doCompletionRequest signature and add logging**

In `internal/bench/completion.go`, change the function (lines 238-377). New signature and body — the full replacement:

```go
func doCompletionRequest(
	ctx context.Context,
	client *http.Client,
	provider ProviderConfig,
	cfg BenchConfig,
	testText string,
	actualInputTokens int,
	tkm *tiktoken.Tiktoken,
	reqID string,
	logger *RequestLogger,
) completionResult {
	res := completionResult{InputTokens: actualInputTokens}

	messages := []completionMessage{}
	if cfg.SystemPrompt != "" {
		messages = append(messages, completionMessage{Role: "system", Content: cfg.SystemPrompt})
	}
	messages = append(messages, completionMessage{Role: "user", Content: testText})

	payload := completionRequest{
		Model:         provider.Model,
		Messages:      messages,
		Stream:        true,
		StreamOptions: &streamOptionsRequest{IncludeUsage: true},
	}
	if cfg.MaxOutputTokens > 0 {
		payload.MaxTokens = cfg.MaxOutputTokens
	}

	body, err := json.Marshal(payload)
	if err != nil {
		res.Err = fmt.Errorf("failed to marshal request body: %w", err)
		return res
	}
	body = MergeCustomParams(body, provider.CustomParams)

	req, err := http.NewRequestWithContext(ctx, "POST", provider.URL, bytes.NewBuffer(body))
	if err != nil {
		res.Err = fmt.Errorf("failed to create request: %w", err)
		return res
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	// LogRequest runs before sendTime so logging can never inflate TTFT/E2E.
	if logger != nil {
		logger.LogRequest(reqID, req.Method, provider.URL, req.Header, body)
	}

	sendTime := time.Now()

	// Response logging state, captured for the deferred LogResponse call which
	// runs after all timing points (TTFT/E2E) have been recorded.
	var (
		respStatus  string
		respHeaders http.Header
		respChunks  []string
		errBodyText string
	)
	if logger != nil {
		defer func() {
			logger.LogResponse(reqID, respStatus, respHeaders, respChunks, errBodyText, res.Err, time.Since(sendTime))
		}()
	}

	resp, err := client.Do(req)
	if err != nil {
		res.Err = err
		return res
	}
	defer resp.Body.Close()

	respStatus = resp.Status
	respHeaders = resp.Header

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		errBodyText = strings.TrimSpace(string(errBody))
		if len(errBodyText) > 0 {
			res.Err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, errBodyText)
		} else {
			res.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return res
	}

	var (
		firstTokenTime time.Time
		lastTokenTime  time.Time
		outputBuf      strings.Builder
		gotFirstToken  bool
		gotContent     bool
		skippedChunks  int
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if logger != nil && line != "" {
			respChunks = append(respChunks, line)
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		if data == "" {
			continue
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			skippedChunks++
			continue
		}
		if chunk.Usage != nil {
			applyAPIUsage(&res, chunk.Usage)
		}

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
	}

	if err := scanner.Err(); err != nil {
		res.Err = fmt.Errorf("failed to read stream: %w", err)
		return res
	}

	if !gotContent {
		res.Err = fmt.Errorf("no output tokens received")
		return res
	}

	res.TTFT = firstTokenTime.Sub(sendTime)
	res.E2E = lastTokenTime.Sub(sendTime)

	outputText := outputBuf.String()
	res.OutputTokens = len(tkm.Encode(outputText, nil, nil))
	if res.OutputTokens == 0 {
		res.OutputTokens = 1
	}
	res.SkippedChunks = skippedChunks
	return res
}
```

Key invariants (do not regress):
- `sendTime := time.Now()` stays immediately before `client.Do`, after `LogRequest`.
- The deferred `LogResponse` runs after `res.TTFT`/`res.E2E` are computed on the success path, and reads the local `res` variable (correct because every `return res` uses the current local value).

- [ ] **Step 4: Update the other call sites**

`internal/bench/completion.go:121` (inside `RunCompletionBench`):

```go
				res := doCompletionRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm, "", nil)
```

`internal/bench/openloop.go:108` (inside `RunOpenLoopCompletionBench`):

```go
			res := doCompletionRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm, "", nil)
```

`internal/bench/completion_ttft_test.go:74` (in `doSingleCompletion`):

```go
	return doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "", nil)
```

`internal/bench/completion_ttft_test.go:294` (in `TestDoCompletionRequest_WirePayload`):

```go
	res := doCompletionRequest(context.Background(), client, provider, cfg, "test prompt", 2, tkm, "", nil)
```

- [ ] **Step 5: Run all bench tests**

Run: `go test ./internal/bench/ -v`
Expected: PASS — all existing tests (TTFT, wire payload, openloop, cache hit, error classification, tiktoken) plus the 3 new wiring tests.

- [ ] **Step 6: Commit**

```bash
git add internal/bench/completion.go internal/bench/openloop.go internal/bench/completion_ttft_test.go internal/bench/request_log_test.go
git commit -m "feat: log request/response in doCompletionRequest"
```

---

### Task 4: Wire logger into RunCompletionBench (closed-loop)

**Files:**
- Modify: `internal/bench/completion.go:69-236` (RunCompletionBench)
- Test: `internal/bench/request_log_test.go` (add runner test)

- [ ] **Step 1: Write the failing runner test**

Append to `internal/bench/request_log_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run 'TestRunCompletionBench_Logs' -v`
Expected: FAIL — `report.LogRequestsFile` empty (logger not wired), files not created.

- [ ] **Step 3: Wire the logger into RunCompletionBench**

In `internal/bench/completion.go`, at the top of `RunCompletionBench` (right after the function signature, before `results := make(...)`):

```go
	var logger *RequestLogger
	runID := time.Now().Format("20060102-150405")
	if cfg.RequestLogging {
		l, err := NewRequestLogger(requestLogDir, runID)
		if err != nil {
			return CompletionReport{
				TotalRequests:   cfg.TotalRequests,
				ErrorDetails:    map[string]int{fmt.Sprintf("failed to initialize request logging: %v", err): 1},
				ErrorCategories: map[string]int{ErrorCategoryClient: 1},
				Valid:           false,
			}
		}
		logger = l
	}
	var reqSeq atomic.Int64
```

Inside the worker loop, change the request call (was updated to `"", nil` in Task 3) to:

```go
				reqID := ""
				if logger != nil {
					reqID = fmt.Sprintf("%s-%06d", runID, reqSeq.Add(1))
				}
				res := doCompletionRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm, reqID, logger)
```

After `wg.Wait()` and `close(results)` (before `wallTime := ...`):

```go
	if logger != nil {
		logger.Close()
	}
```

After the report is built (just before `return report`):

```go
	if logger != nil {
		report.LogRequestsFile = logger.RequestsFile()
		report.LogResponsesFile = logger.ResponsesFile()
		report.LogDroppedCount = int(logger.DroppedCount())
		if err := logger.Err(); err != nil {
			report.LogError = err.Error()
		}
	}
```

Add `"sync/atomic"` to the imports of `completion.go` (`fmt` and `time` are already imported).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/bench/ -run 'TestRunCompletionBench' -v`
Expected: PASS (both new tests).

- [ ] **Step 5: Commit**

```bash
git add internal/bench/completion.go internal/bench/request_log_test.go
git commit -m "feat: create request logger in closed-loop completion bench"
```

---

### Task 5: Wire logger into RunOpenLoopCompletionBench (open-loop)

**Files:**
- Modify: `internal/bench/openloop.go:35-235` (RunOpenLoopCompletionBench)
- Test: `internal/bench/request_log_test.go` (add open-loop test)

- [ ] **Step 1: Write the failing test**

Append to `internal/bench/request_log_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run 'TestRunOpenLoopCompletionBench_Logs' -v`
Expected: FAIL — `report.LogRequestsFile` empty / files missing.

- [ ] **Step 3: Wire the logger into RunOpenLoopCompletionBench**

In `internal/bench/openloop.go`, at the top of `RunOpenLoopCompletionBench` (before `results := make(...)`):

```go
	var logger *RequestLogger
	runID := time.Now().Format("20060102-150405")
	if cfg.RequestLogging {
		l, err := NewRequestLogger(requestLogDir, runID)
		if err != nil {
			return CompletionReport{
				TotalRequests:   cfg.TotalRequests,
				ErrorDetails:    map[string]int{fmt.Sprintf("failed to initialize request logging: %v", err): 1},
				ErrorCategories: map[string]int{ErrorCategoryClient: 1},
				Valid:           false,
			}
		}
		logger = l
	}
	var reqSeq atomic.Int64
```

In the generator, right after `sendTime := time.Now()` / `queueTime` computation (before `wg.Add(1)`), generate the ID and pass it into the request goroutine:

```go
		reqID := ""
		if logger != nil {
			reqID = fmt.Sprintf("%s-%06d", runID, reqSeq.Add(1))
		}

		wg.Add(1)
		go func(qt time.Duration, id string) {
			defer wg.Done()
			defer func() { <-semaphore }() // release slot

			res := doCompletionRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm, id, logger)
			res.QueueTime = qt
			results <- res
			// ... rest unchanged ...
		}(queueTime, reqID)
```

(Only the goroutine signature line, the `doCompletionRequest` call, and the closing line change; the body between is untouched.)

After `wg.Wait()` and `close(results)` (before `wallTime := ...`):

```go
	if logger != nil {
		logger.Close()
	}
```

Before `return report`:

```go
	if logger != nil {
		report.LogRequestsFile = logger.RequestsFile()
		report.LogResponsesFile = logger.ResponsesFile()
		report.LogDroppedCount = int(logger.DroppedCount())
		if err := logger.Err(); err != nil {
			report.LogError = err.Error()
		}
	}
```

Add `"fmt"` and `"sync/atomic"` to the imports of `openloop.go` (`time` already imported).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/bench/ -v`
Expected: PASS — all tests including the new open-loop logging test and all pre-existing openloop tests.

- [ ] **Step 5: Commit**

```bash
git add internal/bench/openloop.go internal/bench/request_log_test.go
git commit -m "feat: create request logger in open-loop completion bench"
```

---

### Task 6: Config screen Request Logging toggle

**Files:**
- Modify: `internal/tui/config_screen.go` (state, buildFieldDefs, toggleFocusedField, renderToggle, validate)
- Test: `internal/tui/config_screen_test.go` (new tests + update 2 existing label-order assertions)

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/config_screen_test.go`:

```go
func TestBuildFieldDefs_RequestLoggingToggleVisibility(t *testing.T) {
	completionSingle := buildFieldDefs(bench.ModeCompletion, "single", 0)
	found := false
	for _, d := range completionSingle {
		if d.label == "Request Logging" {
			found = true
			if d.fieldType != "toggle" {
				t.Error("Request Logging should have fieldType \"toggle\"")
			}
		}
	}
	if !found {
		t.Error("completion single-provider should include Request Logging toggle")
	}

	completionPK := buildFieldDefs(bench.ModeCompletion, "pk", 0)
	if containsLabel(fieldLabels(completionPK), "Request Logging") {
		t.Error("PK mode should not include Request Logging toggle")
	}

	anthropic := buildFieldDefs(bench.ModeAnthropicMessages, "single", 0)
	if containsLabel(fieldLabels(anthropic), "Request Logging") {
		t.Error("anthropic mode should not include Request Logging toggle")
	}

	embedding := buildFieldDefs(bench.ModeEmbedding, "single", 0)
	if containsLabel(fieldLabels(embedding), "Request Logging") {
		t.Error("embedding mode should not include Request Logging toggle")
	}
}

func TestValidate_RequestLogging(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	if m.requestLoggingOn {
		t.Error("expected requestLoggingOn default false")
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
	if cfg.RequestLogging {
		t.Error("expected cfg.RequestLogging=false by default")
	}

	m.requestLoggingOn = true
	_, cfg, err = m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.RequestLogging {
		t.Error("expected cfg.RequestLogging=true after toggle on")
	}
}

func TestToggleFocusedField_RequestLoggingDispatch(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	for i, fd := range m.fieldDefs {
		if fd.label == "Request Logging" {
			m.focusIndex = i
			break
		}
	}
	if m.fieldDefs[m.focusIndex].label != "Request Logging" {
		t.Fatal("Request Logging field not found")
	}
	m.toggleFocusedField()
	if !m.requestLoggingOn {
		t.Error("expected requestLoggingOn=true after toggle")
	}
	if m.loadModelIndex != 0 {
		t.Error("Request Logging toggle should not affect loadModelIndex")
	}
	if !m.ttftReasoningOn {
		t.Error("Request Logging toggle should not affect ttftReasoningOn")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run 'RequestLogging' -v`
Expected: FAIL — compile error: `m.requestLoggingOn undefined (type configModel has no field or method requestLoggingOn)` (until the implementation lands).

- [ ] **Step 3: Implement the toggle**

In `internal/tui/config_screen.go`:

1. Add state to `configModel` (after `ttftReasoningOn`):

```go
	requestLoggingOn bool // Request Logging toggle (completion single-provider only)
```

2. In `buildFieldDefs`, add a field definition near the other toggle defs:

```go
	// Request Logging toggle field (completion single-provider only)
	requestLoggingField := fieldDef{
		label:     "Request Logging",
		fieldType: "toggle",
	}
```

3. In the **single provider** branch of `buildFieldDefs` (the non-PK path at the end of the function), change the final `if isCompletionLike` block to append the toggle after System Prompt, for completion only:

```go
	if isCompletionLike {
		defs = append(defs,
			fieldDef{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
			ttftReasoningField,
			fieldDef{label: "System Prompt", placeholder: "optional, leave blank to skip"},
		)
	}
	if isCompletion {
		defs = append(defs, requestLoggingField)
	}
	return defs
```

(Do not touch the PK branch — the toggle must not appear there.)

4. In `toggleFocusedField`, add a case:

```go
	case "Request Logging":
		m.requestLoggingOn = !m.requestLoggingOn
```

5. In `renderToggle`, add a case:

```go
	case "Request Logging":
		if m.requestLoggingOn {
			return "( ) off  (●) on"
		}
		return "(●) off  ( ) on"
```

6. In `validate()`, single-provider path: after the `if isCompletionLike { ... }` block that sets MaxOutputTokens/SystemPrompt/TTFTIncludesReasoning, add:

```go
	if isCompletion {
		cfg.RequestLogging = m.requestLoggingOn
	}
```

- [ ] **Step 4: Update the two existing label-order assertions**

In `internal/tui/config_screen_test.go`:

`TestBuildFieldDefs_CompletionClosedLoop` — expected list becomes:

```go
	assertLabelOrder(t, defs, []string{
		"API URL", "API Key", "Model", "Custom Params",
		"Load Model", "Concurrency", "Total Requests", "Input Tokens",
		"Max Output Tokens", "TTFT Includes Reasoning", "System Prompt",
		"Request Logging",
	})
```

`TestBuildFieldDefs_CompletionOpenLoop` — expected list becomes:

```go
	assertLabelOrder(t, defs, []string{
		"API URL", "API Key", "Model", "Custom Params",
		"Load Model", "Max In-Flight", "Request Rate", "Total Requests", "Input Tokens",
		"Max Output Tokens", "TTFT Includes Reasoning", "System Prompt",
		"Request Logging",
	})
```

- [ ] **Step 5: Run all TUI tests**

Run: `go test ./internal/tui/ -v`
Expected: PASS — all existing and new tests.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/config_screen.go internal/tui/config_screen_test.go
git commit -m "feat: add Request Logging toggle to config screen"
```

---

### Task 7: Results screen log paths + drop/error warning

**Files:**
- Modify: `internal/tui/results_screen.go:293-382` (renderCompletionSingle)
- Test: `internal/tui/results_screen_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/tui/results_screen_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"embedding_benchmark/internal/bench"
)

func TestRenderCompletionSingle_ShowsLogInfo(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"Provider"})
	m.addCompletionResult(0, &bench.CompletionReport{
		TotalRequests:    3,
		SuccessCount:     3,
		WallTime:         1e9, // 1s in nanoseconds
		LogRequestsFile:  "bench_requests_20260729-153045.jsonl",
		LogResponsesFile: "bench_responses_20260729-153045.jsonl",
		LogDroppedCount:  12,
		LogError:         "disk full",
		Valid:            true,
	})
	var sb strings.Builder
	m.renderCompletionSingle(&sb)
	out := sb.String()
	if !strings.Contains(out, "bench_requests_20260729-153045.jsonl") {
		t.Error("results should show the requests log file path")
	}
	if !strings.Contains(out, "bench_responses_20260729-153045.jsonl") {
		t.Error("results should show the responses log file path")
	}
	if !strings.Contains(out, "12 log entries dropped") {
		t.Error("results should warn about dropped log entries")
	}
	if !strings.Contains(out, "disk full") {
		t.Error("results should show the log write error")
	}
}

func TestRenderCompletionSingle_NoLogInfoWhenDisabled(t *testing.T) {
	m := newResultsModel(bench.ModeCompletion, "single", []string{"Provider"})
	m.addCompletionResult(0, &bench.CompletionReport{
		TotalRequests: 1,
		SuccessCount:  1,
		WallTime:      1e9,
		Valid:         true,
	})
	var sb strings.Builder
	m.renderCompletionSingle(&sb)
	out := sb.String()
	if strings.Contains(out, "Logs:") {
		t.Error("results should not show log paths when logging disabled")
	}
}
```

Note: `WallTime: 1e9` is a `time.Duration` (nanoseconds) = 1 second.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestRenderCompletionSingle' -v`
Expected: FAIL — "results should show the requests log file path".

- [ ] **Step 3: Implement the display**

In `internal/tui/results_screen.go`, in `renderCompletionSingle`, immediately after the Queue Time section block (after its closing `}`) and before the `if r.ErrorCount > 0` block, add:

```go
	if r.LogRequestsFile != "" {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("  Logs: %s / %s", r.LogRequestsFile, r.LogResponsesFile)))
		sb.WriteString("\n")
		if r.LogDroppedCount > 0 {
			sb.WriteString(errorStyle.Render(fmt.Sprintf("  ⚠ %d log entries dropped (disk too slow)", r.LogDroppedCount)))
			sb.WriteString("\n")
		}
		if r.LogError != "" {
			sb.WriteString(errorStyle.Render(fmt.Sprintf("  ⚠ log write error: %s", r.LogError)))
			sb.WriteString("\n")
		}
	}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/tui/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/results_screen.go internal/tui/results_screen_test.go
git commit -m "feat: show request log paths and drop warnings on results screen"
```

---

### Task 8: .gitignore, AGENTS.md, final verification

**Files:**
- Modify: `.gitignore`
- Modify: `AGENTS.md`

- [ ] **Step 1: Ignore log files**

Append to `.gitignore` (follow the existing Chinese-comment style):

```gitignore
# 压测请求日志
bench_requests_*.jsonl
bench_responses_*.jsonl
```

- [ ] **Step 2: Document in AGENTS.md**

In `AGENTS.md`, under `### Key Design Decisions`, add one bullet (after the TTFT semantics bullet):

```markdown
- **Request logging**: Chat Completion single-provider benchmarks can enable `Request Logging` (config toggle, default off). Every request/response (headers + exact wire body + raw SSE lines) is written to `bench_requests_<runID>.jsonl` / `bench_responses_<runID>.jsonl` in the working directory, correlated by `request_id` (`<runID>-<6-digit seq>`, local only). Fully asynchronous: request goroutines enqueue raw data via non-blocking sends (drop-on-full, counted in `CompletionReport.LogDroppedCount`), and all serialization/disk IO happens in a single writer goroutine — logging never delays TTFT/E2E/TPOT. The `Authorization` header is redacted (`Bearer ***last4`). Results screen shows the log paths plus drop/write-error warnings.
```

Also in `AGENTS.md`, in the `### Key Types in internal/bench` section, update the `BenchConfig` snippet to include the new field (add one line after `TTFTIncludesReasoning`):

```go
    RequestLogging bool      // completion single-provider: async request/response JSONL logging
```

- [ ] **Step 3: Full verification**

Run: `go build ./... && go vet ./... && go test ./... -count=1`
Expected: build OK, vet OK, all tests PASS.

- [ ] **Step 4: Smoke check the gitignore**

Run: `touch bench_requests_test999.jsonl bench_responses_test999.jsonl && git status --porcelain && rm bench_requests_test999.jsonl bench_responses_test999.jsonl`
Expected: `git status --porcelain` prints nothing for these two files (they are ignored).

- [ ] **Step 5: Commit**

```bash
git add .gitignore AGENTS.md
git commit -m "docs: document request logging; ignore JSONL log files"
```

---

## Self-Review Notes

- **Spec coverage**: toggle (Task 6) ✓, request ID format (Tasks 4-5) ✓, two JSONL files + naming (Task 1) ✓, complete body + headers + redaction (Tasks 1, 3) ✓, raw SSE chunks (Task 3) ✓, non-200/error entries (Task 3) ✓, async non-blocking + drop counter (Task 1) ✓, init-failure policy (Tasks 4-5) ✓, report fields (Task 2) ✓, results display (Task 7) ✓, .gitignore/AGENTS.md (Task 8) ✓, tests (every task) ✓.
- **Type consistency**: `requestLogEntry`/`responseLogEntry`/`logEntry`/`RequestLogger` defined in Task 1, used in Tasks 3-5; `LogRequest(reqID, method, url, headerMap, body)` and `LogResponse(reqID, status, headerMap, chunks, errBody, reqErr, dur)` signatures match between definition and call sites; `redactAuthorization`/`flattenHeaders` match their tests; `requestLogDir` package var matches test overrides; report fields `LogRequestsFile`/`LogResponsesFile`/`LogDroppedCount`/`LogError` match between stats.go, runners, results screen, and tests.
