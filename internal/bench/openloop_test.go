package bench

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// mockCompletionServer returns a streaming SSE server that sends a fixed response.
func mockCompletionServer(t *testing.T) *httptest.Server {
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

func TestRunOpenLoopCompletionBench_Basic(t *testing.T) {
	server := mockCompletionServer(t)
	defer server.Close()

	tkm := testTokenizer(t)

	provider := ProviderConfig{
		URL:    server.URL,
		Model:  "test-model",
		APIKey: "",
	}
	cfg := BenchConfig{
		Mode:          ModeCompletion,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 5,
		RequestRate:   100, // high rate so test runs fast
		MaxInFlight:   3,
	}

	report := RunOpenLoopCompletionBench(
		context.Background(), provider, cfg, []string{"test prompt"}, 2, tkm, nil,
	)

	if report.TotalRequests != 5 {
		t.Errorf("expected TotalRequests=5, got %d", report.TotalRequests)
	}
	if report.SuccessCount != 5 {
		t.Errorf("expected SuccessCount=5, got %d (errors: %v)", report.SuccessCount, report.ErrorDetails)
	}
	if !report.Valid {
		t.Error("expected report to be valid")
	}
}

func TestRunOpenLoopCompletionBench_RespectsSemaphore(t *testing.T) {
	var maxConcurrent int64
	var currentConcurrent int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := atomic.AddInt64(&currentConcurrent, 1)
		for {
			old := atomic.LoadInt64(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt64(&maxConcurrent, old, cur) {
				break
			}
		}
		defer atomic.AddInt64(&currentConcurrent, -1)

		// Hold the request briefly so concurrency can build up
		time.Sleep(50 * time.Millisecond)

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

	maxInFlight := 2
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:          ModeCompletion,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 6,
		RequestRate:   1000, // very high rate so requests arrive almost simultaneously
		MaxInFlight:   maxInFlight,
	}

	report := RunOpenLoopCompletionBench(
		context.Background(), provider, cfg, []string{"test"}, 1, tkm, nil,
	)

	if report.SuccessCount != 6 {
		t.Fatalf("expected SuccessCount=6, got %d (errors: %v)", report.SuccessCount, report.ErrorDetails)
	}
	observed := atomic.LoadInt64(&maxConcurrent)
	if observed > int64(maxInFlight) {
		t.Errorf("max concurrent requests %d exceeded MaxInFlight %d", observed, maxInFlight)
	}
}

func TestRunOpenLoopCompletionBench_Cancellation(t *testing.T) {
	server := mockCompletionServer(t)
	defer server.Close()

	tkm := testTokenizer(t)

	ctx, cancel := context.WithCancel(context.Background())

	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:          ModeCompletion,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 100,
		RequestRate:   1, // very slow — 1 req/s
		MaxInFlight:   5,
	}

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	report := RunOpenLoopCompletionBench(ctx, provider, cfg, []string{"test"}, 1, tkm, nil)
	elapsed := time.Since(start)

	// Cancellation stops the generator: far fewer than 100 requests are sent,
	// and requests that were never sent are dropped rather than recorded.
	total := report.SuccessCount + report.ErrorCount
	if total >= 100 {
		t.Errorf("expected fewer than 100 completed+error requests after cancel, got %d", total)
	}
	if elapsed > 10*time.Second {
		t.Errorf("expected prompt return after cancel, took %v", elapsed)
	}
}

func TestRunOpenLoopCompletionBench_ZeroTotalRequests(t *testing.T) {
	server := mockCompletionServer(t)
	defer server.Close()

	tkm := testTokenizer(t)

	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:          ModeCompletion,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 0,
		RequestRate:   10,
		MaxInFlight:   5,
	}

	report := RunOpenLoopCompletionBench(
		context.Background(), provider, cfg, []string{"test"}, 1, tkm, nil,
	)

	if report.TotalRequests != 0 {
		t.Errorf("expected TotalRequests=0, got %d", report.TotalRequests)
	}
	if report.Valid {
		t.Error("expected report to be invalid with 0 requests")
	}
}

func TestPoissonInterval(t *testing.T) {
	rate := 10.0
	// Generate many intervals and check the mean is approximately 1/rate
	n := 10000
	var sum float64
	for i := 0; i < n; i++ {
		iv := poissonInterval(rate)
		if iv < 0 {
			t.Fatalf("interval = %v, want >= 0", iv)
		}
		sum += iv.Seconds()
	}
	mean := sum / float64(n)
	expected := 1.0 / rate
	// Allow 10% tolerance for statistical variation
	if mean < expected*0.9 || mean > expected*1.1 {
		t.Errorf("mean interval %f too far from expected %f", mean, expected)
	}
}

func TestRunOpenLoopCompletionBench_QueueTimeNonNegative(t *testing.T) {
	// Slow server + MaxInFlight=1 + high arrival rate forces requests to queue.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	tkm := testTokenizer(t)

	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:          ModeCompletion,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 5,
		RequestRate:   10000, // arrivals much faster than the 20ms service time
		MaxInFlight:   1,     // force queuing
	}

	report := RunOpenLoopCompletionBench(
		context.Background(), provider, cfg, []string{"test"}, 1, tkm, nil,
	)

	if report.SuccessCount != 5 {
		t.Fatalf("expected 5 successful requests, got %d (errors: %v)", report.SuccessCount, report.ErrorDetails)
	}
	// QueueTimeAvg should be >= 0 (no negative values)
	if report.QueueTimeAvg < 0 {
		t.Errorf("QueueTimeAvg should be >= 0, got %f", report.QueueTimeAvg)
	}
	// With forced queuing, later requests visibly wait for the semaphore slot.
	if report.QueueTimeAvg == 0 {
		t.Error("QueueTimeAvg should be > 0 when requests queue behind MaxInFlight=1")
	}
}

func mockAnthropicMessagesServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":1}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
}

func TestRunOpenLoopCompletionBench_Anthropic(t *testing.T) {
	server := mockAnthropicMessagesServer(t)
	defer server.Close()

	tkm := testTokenizer(t)
	provider := ProviderConfig{URL: server.URL, Model: "claude-test", APIKey: "sk-ant-test"}
	cfg := BenchConfig{
		Mode:          ModeAnthropicMessages,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 3,
		RequestRate:   1000,
		MaxInFlight:   2,
	}

	report := RunOpenLoopCompletionBench(
		context.Background(), provider, cfg, []string{"test"}, 1, tkm, nil,
	)
	if report.SuccessCount != 3 {
		t.Fatalf("expected 3 successful requests, got %d (errors: %v)", report.SuccessCount, report.ErrorDetails)
	}
	if report.QueueTimeAvg < 0 {
		t.Errorf("QueueTimeAvg should be >= 0, got %f", report.QueueTimeAvg)
	}
}
