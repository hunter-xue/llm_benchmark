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
	RequestID  string            `json:"request_id"`
	TS         string            `json:"ts"`
	DurationMs int64             `json:"duration_ms"`
	Status     string            `json:"status,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Chunks     []string          `json:"chunks,omitempty"`
	Body       string            `json:"body,omitempty"` // non-200 error body snippet
	Error      string            `json:"error,omitempty"`
}

// logEntry carries one entry to the writer goroutine; exactly one field is set.
type logEntry struct {
	req  *requestLogEntry
	resp *responseLogEntry
}

// RequestLogger asynchronously writes request/response JSONL files.
// Safe for concurrent use. Enqueue never blocks: entries are dropped
// (and counted) when the writer falls behind.
//
// Lifecycle: producers must stop calling LogRequest/LogResponse before
// Close is called. Close is not idempotent.
type RequestLogger struct {
	requestsFile  string
	responsesFile string

	ch      chan logEntry
	wg      sync.WaitGroup
	dropped atomic.Int64

	errMu    sync.Mutex
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

// LogRequest enqueues a request entry. headerMap is cloned and Authorization /
// x-api-key are redacted synchronously (cheap, happens before the request is sent).
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
		RequestID:  reqID,
		TS:         logTimestamp(),
		DurationMs: dur.Milliseconds(),
		Status:     status,
		Headers:    flattenHeaders(headerMap),
		Chunks:     chunks,
		Body:       errBody,
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
	if err := l.reqFile.Close(); err != nil {
		l.setErr(err)
	}
	if err := l.respFile.Close(); err != nil {
		l.setErr(err)
	}
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

// flattenHeaders converts http.Header to a single-valued map and redacts
// Authorization and x-api-key credentials. The input map is not modified.
func flattenHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	flat := make(map[string]string, len(h))
	for k, vals := range h {
		v := strings.Join(vals, ", ")
		if strings.EqualFold(k, "Authorization") {
			v = redactAuthorization(v)
		} else if strings.EqualFold(k, "x-api-key") {
			v = maskCredential(v)
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
