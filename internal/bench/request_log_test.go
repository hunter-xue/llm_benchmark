package bench

import (
	"encoding/json"
	"errors"
	"net/http"
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
