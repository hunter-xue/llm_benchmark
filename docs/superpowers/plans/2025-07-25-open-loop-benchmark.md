# Open-Loop Benchmark Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an open-loop (SGLang-style) concurrency model to the Chat Completion benchmark with Poisson request arrival, semaphore-based max in-flight control, and queue time metrics.

**Architecture:** New file `internal/bench/openloop.go` contains `RunOpenLoopCompletionBench` with a generator goroutine producing requests at Poisson intervals and a buffered-channel semaphore limiting concurrency. The TUI config screen gains a radio toggle for Load Model selection, dynamically showing/hiding concurrency fields. Results display new Queue Time metrics.

**Tech Stack:** Go, bubbletea TUI, existing `doCompletionRequest` for HTTP execution.

---

### Task 1: Add QueueTime to completionResult and CompletionReport

**Files:**
- Modify: `internal/bench/completion.go:52-63` (completionResult struct)
- Modify: `internal/bench/stats.go:27-59` (CompletionReport struct)

- [ ] **Step 1: Add QueueTime field to completionResult**

In `internal/bench/completion.go`, add one field to the struct:

```go
type completionResult struct {
	TTFT          time.Duration
	E2E           time.Duration
	QueueTime     time.Duration // open-loop only: time from generation to actual send
	InputTokens   int
	OutputTokens  int
	APIPrompt     int
	APICompletion int
	APITotal      int
	HasAPIUsage   bool
	SkippedChunks int
	Err           error
}
```

- [ ] **Step 2: Add QueueTime fields to CompletionReport**

In `internal/bench/stats.go`, add four fields:

```go
type CompletionReport struct {
	TotalRequests        int
	SuccessCount         int
	ErrorCount           int
	WallTime             time.Duration
	RPS                  float64
	InputTPS             float64
	OutputTPS            float64
	InputTPM             float64
	OutputTPM            float64
	AvgOutputTokens      float64
	APIPromptTokens      int
	APICompletionTokens  int
	APITotalTokens       int
	APIUsageCount        int
	MissingAPIUsageCount int
	TTFTAvg              float64
	TTFTp50              float64
	TTFTp90              float64
	TTFTp99              float64
	TPOTAvg              float64
	TPOTp50              float64
	TPOTp90              float64
	TPOTp99              float64
	E2EAvg               float64
	E2Ep50               float64
	E2Ep90               float64
	E2Ep99               float64
	QueueTimeAvg         float64 // open-loop only
	QueueTimeP50         float64 // open-loop only
	QueueTimeP90         float64 // open-loop only
	QueueTimeP99         float64 // open-loop only
	SkippedChunks        int
	ErrorDetails         map[string]int
	ErrorCategories      map[string]int
	Valid                bool
}
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 4: Commit**

```bash
git add internal/bench/completion.go internal/bench/stats.go
git commit -m "feat: add QueueTime fields to completionResult and CompletionReport"
```

---

### Task 2: Add LoadModel, RequestRate, MaxInFlight to BenchConfig

**Files:**
- Modify: `internal/bench/config.go:12-35`

- [ ] **Step 1: Add constants and fields**

In `internal/bench/config.go`, add load model constants after the mode constants:

```go
const (
	ModeEmbedding         = "embedding"
	ModeCompletion        = "completion"
	ModeAnthropicMessages = "anthropic_messages"
)

const (
	LoadModelClosedLoop = "closed_loop"
	LoadModelOpenLoop   = "open_loop"
)
```

Add three fields to `BenchConfig`:

```go
type BenchConfig struct {
	Mode            string
	Concurrency     int
	TotalRequests   int
	TargetTokens    int
	MaxOutputTokens int
	SystemPrompt    string
	LoadModel       string // "closed_loop" (default) or "open_loop"
	RequestRate     int    // open-loop: requests per second (Poisson rate)
	MaxInFlight     int    // open-loop: max concurrent in-flight requests
}
```

- [ ] **Step 2: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add internal/bench/config.go
git commit -m "feat: add LoadModel, RequestRate, MaxInFlight to BenchConfig"
```

---

### Task 3: Create openloop.go — core scheduler

**Files:**
- Create: `internal/bench/openloop.go`

- [ ] **Step 1: Write failing test**

Create `internal/bench/openloop_test.go`:

```go
package bench

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tiktoken "github.com/pkoukk/tiktoken-go"
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

	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		t.Fatalf("failed to get encoding: %v", err)
	}

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
		context.Background(), provider, cfg, "test prompt", 2, tkm, nil,
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

	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		t.Fatalf("failed to get encoding: %v", err)
	}

	maxInFlight := 2
	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:          ModeCompletion,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 6,
		RequestRate:   1000, // very high rate so requests arrive almost simultaneously
		MaxInFlight:   maxInFlight,
	}

	RunOpenLoopCompletionBench(
		context.Background(), provider, cfg, "test", 1, tkm, nil,
	)

	observed := atomic.LoadInt64(&maxConcurrent)
	if observed > int64(maxInFlight) {
		t.Errorf("max concurrent requests %d exceeded MaxInFlight %d", observed, maxInFlight)
	}
}

func TestRunOpenLoopCompletionBench_Cancellation(t *testing.T) {
	server := mockCompletionServer(t)
	defer server.Close()

	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		t.Fatalf("failed to get encoding: %v", err)
	}

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

	report := RunOpenLoopCompletionBench(ctx, provider, cfg, "test", 1, tkm, nil)

	// Should have far fewer than 100 requests due to cancellation
	total := report.SuccessCount + report.ErrorCount
	if total >= 100 {
		t.Errorf("expected fewer than 100 completed+error requests after cancel, got %d", total)
	}
}

func TestRunOpenLoopCompletionBench_ZeroTotalRequests(t *testing.T) {
	server := mockCompletionServer(t)
	defer server.Close()

	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		t.Fatalf("failed to get encoding: %v", err)
	}

	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:          ModeCompletion,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 0,
		RequestRate:   10,
		MaxInFlight:   5,
	}

	report := RunOpenLoopCompletionBench(
		context.Background(), provider, cfg, "test", 1, tkm, nil,
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
	server := mockCompletionServer(t)
	defer server.Close()

	tkm, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		t.Fatalf("failed to get encoding: %v", err)
	}

	provider := ProviderConfig{URL: server.URL, Model: "test-model"}
	cfg := BenchConfig{
		Mode:          ModeCompletion,
		LoadModel:     LoadModelOpenLoop,
		TotalRequests: 5,
		RequestRate:   100,
		MaxInFlight:   1, // force queuing
	}

	report := RunOpenLoopCompletionBench(
		context.Background(), provider, cfg, "test", 1, tkm, nil,
	)

	if report.SuccessCount == 0 {
		t.Fatal("expected some successful requests")
	}
	// QueueTimeAvg should be >= 0 (no negative values)
	if report.QueueTimeAvg < 0 {
		t.Errorf("QueueTimeAvg should be >= 0, got %f", report.QueueTimeAvg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/bench/ -run TestRunOpenLoop -v -count=1 2>&1 | head -30`
Expected: FAIL — `RunOpenLoopCompletionBench` undefined

- [ ] **Step 3: Write the implementation**

Create `internal/bench/openloop.go`:

```go
package bench

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"time"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

// poissonInterval returns a random inter-arrival duration for a Poisson process
// with the given rate (events per second). Uses the exponential distribution.
func poissonInterval(rate float64) time.Duration {
	// -ln(1 - U) / rate, where U ~ Uniform(0, 1)
	// Using 1 - rand.Float64() to avoid log(0)
	u := 1.0 - rand.Float64()
	if u <= 0 {
		u = 1e-10
	}
	return time.Duration(-math.Log(u) / rate * float64(time.Second))
}

// RunOpenLoopCompletionBench runs an open-loop streaming completion benchmark.
// Requests are generated at Poisson intervals (RequestRate req/s) and limited
// to MaxInFlight concurrent in-flight requests via a semaphore.
func RunOpenLoopCompletionBench(
	ctx context.Context,
	provider ProviderConfig,
	cfg BenchConfig,
	testText string,
	actualInputTokens int,
	tkm *tiktoken.Tiktoken,
	onProgress func(ProgressUpdate),
) CompletionReport {
	results := make(chan completionResult, cfg.TotalRequests)
	semaphore := make(chan struct{}, cfg.MaxInFlight)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		completed int
		errCount  int
	)

	startTime := time.Now()

	// Single HTTP client shared across all request goroutines
	client := &http.Client{Timeout: 300 * time.Second}

	// Generator: produce requests at Poisson intervals
	for i := 0; i < cfg.TotalRequests; i++ {
		if i > 0 {
			interval := poissonInterval(float64(cfg.RequestRate))
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				// Record cancelled requests that were never sent
				remaining := cfg.TotalRequests - i
				for j := 0; j < remaining; j++ {
					results <- completionResult{Err: ctx.Err()}
					mu.Lock()
					errCount++
					c, e := completed, errCount
					mu.Unlock()
					if onProgress != nil {
						onProgress(ProgressUpdate{
							Completed:     c,
							Errors:        e,
							ErrorDetail:   ctx.Err().Error(),
							ErrorCategory: ClassifyError(ctx.Err()),
						})
					}
				}
				goto waitForInFlight
			case <-timer.C:
			}
		}

		generatedAt := time.Now()

		// Acquire semaphore slot (blocks when full)
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			results <- completionResult{Err: ctx.Err()}
			mu.Lock()
			errCount++
			c, e := completed, errCount
			mu.Unlock()
			if onProgress != nil {
				onProgress(ProgressUpdate{
					Completed:     c,
					Errors:        e,
					ErrorDetail:   ctx.Err().Error(),
					ErrorCategory: ClassifyError(ctx.Err()),
				})
			}
			continue
		}

		sendTime := time.Now()
		queueTime := sendTime.Sub(generatedAt)
		if queueTime < 0 {
			queueTime = 0
		}

		wg.Add(1)
		go func(qt time.Duration) {
			defer wg.Done()
			defer func() { <-semaphore }() // release slot

			res := doCompletionRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm)
			res.QueueTime = qt
			results <- res

			mu.Lock()
			if res.Err != nil {
				errCount++
			} else {
				completed++
			}
			c, e := completed, errCount
			mu.Unlock()
			if onProgress != nil {
				update := ProgressUpdate{Completed: c, Errors: e}
				if res.Err != nil {
					update.ErrorDetail = res.Err.Error()
					update.ErrorCategory = ClassifyError(res.Err)
				}
				onProgress(update)
			}
		}(queueTime)
	}

waitForInFlight:
	wg.Wait()
	close(results)
	wallTime := time.Since(startTime)

	// Aggregate results (same logic as RunCompletionBench)
	var (
		ttfts              []float64
		e2es               []float64
		tpots              []float64
		queueTimes         []float64
		totalInToks        int
		totalOutToks       int
		apiPromptToks      int
		apiCompletionToks  int
		apiTotalToks       int
		apiUsageCount      int
		missingAPIUsage    int
		totalSkippedChunks int
		errors             int
		errorDetails       = make(map[string]int)
		errorCategories    = make(map[string]int)
	)

	for r := range results {
		if r.Err != nil {
			errors++
			recordError(errorDetails, errorCategories, r.Err)
		} else {
			ttfts = append(ttfts, float64(r.TTFT.Milliseconds()))
			e2es = append(e2es, float64(r.E2E.Milliseconds()))
			queueTimes = append(queueTimes, float64(r.QueueTime.Milliseconds()))
			totalInToks += r.InputTokens
			totalOutToks += r.OutputTokens
			if r.HasAPIUsage {
				apiPromptToks += r.APIPrompt
				apiCompletionToks += r.APICompletion
				apiTotalToks += r.APITotal
				apiUsageCount++
			} else {
				missingAPIUsage++
			}
			totalSkippedChunks += r.SkippedChunks

			if r.OutputTokens > 1 {
				decodeMs := float64((r.E2E - r.TTFT).Milliseconds())
				tpots = append(tpots, decodeMs/float64(r.OutputTokens-1))
			} else if r.OutputTokens == 1 {
				tpots = append(tpots, float64(r.E2E.Milliseconds()))
			}
		}
	}

	sort.Float64s(ttfts)
	sort.Float64s(e2es)
	sort.Float64s(tpots)
	sort.Float64s(queueTimes)

	successCount := len(e2es)
	report := CompletionReport{
		TotalRequests:   cfg.TotalRequests,
		SuccessCount:    successCount,
		ErrorCount:      errors,
		WallTime:        wallTime,
		SkippedChunks:   totalSkippedChunks,
		ErrorDetails:    errorDetails,
		ErrorCategories: errorCategories,
		Valid:           successCount > 0,
	}
	report.APIPromptTokens = apiPromptToks
	report.APICompletionTokens = apiCompletionToks
	report.APITotalTokens = apiTotalToks
	report.APIUsageCount = apiUsageCount
	report.MissingAPIUsageCount = missingAPIUsage

	if successCount > 0 && wallTime.Seconds() > 0 {
		report.RPS = float64(successCount) / wallTime.Seconds()
		report.InputTPS = float64(totalInToks) / wallTime.Seconds()
		report.OutputTPS = float64(totalOutToks) / wallTime.Seconds()
		report.InputTPM = float64(totalInToks) / wallTime.Minutes()
		report.OutputTPM = float64(totalOutToks) / wallTime.Minutes()
		report.AvgOutputTokens = float64(totalOutToks) / float64(successCount)
		report.TTFTAvg = average(ttfts)
		report.TTFTp50 = percentile(ttfts, 0.50)
		report.TTFTp90 = percentile(ttfts, 0.90)
		report.TTFTp99 = percentile(ttfts, 0.99)
		report.E2EAvg = average(e2es)
		report.E2Ep50 = percentile(e2es, 0.50)
		report.E2Ep90 = percentile(e2es, 0.90)
		report.E2Ep99 = percentile(e2es, 0.99)
		if len(tpots) > 0 {
			report.TPOTAvg = average(tpots)
			report.TPOTp50 = percentile(tpots, 0.50)
			report.TPOTp90 = percentile(tpots, 0.90)
			report.TPOTp99 = percentile(tpots, 0.99)
		}
		if len(queueTimes) > 0 {
			report.QueueTimeAvg = average(queueTimes)
			report.QueueTimeP50 = percentile(queueTimes, 0.50)
			report.QueueTimeP90 = percentile(queueTimes, 0.90)
			report.QueueTimeP99 = percentile(queueTimes, 0.99)
		}
	}
	return report
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/bench/ -run TestRunOpenLoop -v -count=1 -timeout 30s`
Expected: all tests PASS

Run: `go test ./internal/bench/ -run TestPoissonInterval -v -count=1 -timeout 30s`
Expected: PASS

- [ ] **Step 5: Run all existing tests to verify no regression**

Run: `go test ./internal/bench/ -v -count=1 -timeout 60s`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/bench/openloop.go internal/bench/openloop_test.go
git commit -m "feat: add open-loop scheduler with Poisson arrival and semaphore concurrency control"
```

---

### Task 4: Update config screen — Load Model toggle and dynamic fields

**Files:**
- Modify: `internal/tui/config_screen.go`

- [ ] **Step 1: Write failing test**

Create `internal/tui/config_screen_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"embedding_benchmark/internal/bench"
)

func TestBuildFieldDefs_CompletionClosedLoop(t *testing.T) {
	defs := buildFieldDefs(bench.ModeCompletion, "single")
	labels := fieldLabels(defs)

	// closed-loop default: should have Concurrency, not Max In-Flight or Request Rate
	if !containsLabel(labels, "Concurrency") {
		t.Error("closed-loop should include Concurrency field")
	}
	if containsLabel(labels, "Max In-Flight") {
		t.Error("closed-loop should not include Max In-Flight field")
	}
	if containsLabel(labels, "Request Rate") {
		t.Error("closed-loop should not include Request Rate field")
	}
	if containsLabel(labels, "Load Model") {
		t.Error("closed-loop should not include Load Model toggle (it's implicit)")
	}
}

func TestBuildFieldDefs_CompletionOpenLoop(t *testing.T) {
	defs := buildFieldDefs(bench.ModeCompletion, "single")
	labels := fieldLabels(defs)

	// Simulate what happens after toggle: the caller rebuilds with loadModelIndex=1
	// buildFieldDefs itself always returns the base set; dynamic fields are handled
	// by newConfigModel. For now, verify the base set is correct.
	if !containsLabel(labels, "Concurrency") {
		t.Error("base field defs should include Concurrency")
	}
}

func TestBuildFieldDefs_Embedding(t *testing.T) {
	defs := buildFieldDefs(bench.ModeEmbedding, "single")
	labels := fieldLabels(defs)

	if !containsLabel(labels, "Concurrency") {
		t.Error("embedding should include Concurrency")
	}
	// Embedding mode should never show Load Model toggle
	if containsLabel(labels, "Load Model") {
		t.Error("embedding should not include Load Model toggle")
	}
}

func TestBuildFieldDefs_AnthropicMessages(t *testing.T) {
	defs := buildFieldDefs(bench.ModeAnthropicMessages, "single")
	labels := fieldLabels(defs)

	if !containsLabel(labels, "Concurrency") {
		t.Error("anthropic should include Concurrency")
	}
	// Anthropic Messages mode should never show Load Model toggle
	if containsLabel(labels, "Load Model") {
		t.Error("anthropic should not include Load Model toggle")
	}
}

func TestValidate_OpenLoopRequiresMaxInFlight(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1 // open-loop

	// Fill required fields but leave Max In-Flight empty
	setInputValues(m.inputs, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "",
		"Request Rate":   "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, _, err := m.validate()
	if err == nil {
		t.Error("expected error when Max In-Flight is empty in open-loop mode")
	}
	if !strings.Contains(err.Error(), "Max In-Flight") {
		t.Errorf("error should mention Max In-Flight, got: %v", err)
	}
}

func TestValidate_OpenLoopRequiresRequestRate(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1

	setInputValues(m.inputs, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "10",
		"Request Rate":   "",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, _, err := m.validate()
	if err == nil {
		t.Error("expected error when Request Rate is empty in open-loop mode")
	}
	if !strings.Contains(err.Error(), "Request Rate") {
		t.Errorf("error should mention Request Rate, got: %v", err)
	}
}

func TestValidate_ClosedLoopIgnoresOpenLoopFields(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	// loadModelIndex defaults to 0 (closed-loop)

	setInputValues(m.inputs, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Concurrency":    "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("closed-loop should not require open-loop fields, got: %v", err)
	}
	if cfg.LoadModel != bench.LoadModelClosedLoop {
		t.Errorf("expected LoadModel=closed_loop, got %s", cfg.LoadModel)
	}
}

func TestValidate_OpenLoopSetsConfigFields(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1

	setInputValues(m.inputs, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "5",
		"Request Rate":   "20",
		"Total Requests": "50",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LoadModel != bench.LoadModelOpenLoop {
		t.Errorf("expected LoadModel=open_loop, got %s", cfg.LoadModel)
	}
	if cfg.MaxInFlight != 5 {
		t.Errorf("expected MaxInFlight=5, got %d", cfg.MaxInFlight)
	}
	if cfg.RequestRate != 20 {
		t.Errorf("expected RequestRate=20, got %d", cfg.RequestRate)
	}
	if cfg.TotalRequests != 50 {
		t.Errorf("expected TotalRequests=50, got %d", cfg.TotalRequests)
	}
}

func TestValidate_OpenLoopPKMode(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "pk")
	m.loadModelIndex = 1

	setInputValues(m.inputs, map[string]string{
		"Provider A URL":   "https://api.openai.com/v1/chat/completions",
		"Provider A Model": "gpt-4o-mini",
		"Provider B URL":   "https://api.openai.com/v1/chat/completions",
		"Provider B Model": "gpt-4o",
		"Max In-Flight":    "10",
		"Request Rate":     "30",
		"Total Requests":   "20",
		"Input Tokens":     "100",
	})

	providers, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
	if cfg.LoadModel != bench.LoadModelOpenLoop {
		t.Errorf("expected LoadModel=open_loop, got %s", cfg.LoadModel)
	}
	if cfg.MaxInFlight != 10 {
		t.Errorf("expected MaxInFlight=10, got %d", cfg.MaxInFlight)
	}
	if cfg.RequestRate != 30 {
		t.Errorf("expected RequestRate=30, got %d", cfg.RequestRate)
	}
}

// Helper: extract labels from fieldDefs
func fieldLabels(defs []fieldDef) []string {
	labels := make([]string, len(defs))
	for i, d := range defs {
		labels[i] = d.label
	}
	return labels
}

func containsLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

// Helper: set input values by matching label
func setInputValues(inputs []textinput.Model, values map[string]string) {
	// We need to map field labels to input indices
	// This is a helper that works with the test's knowledge of field order
	// For simplicity, we use a label->value map and set by index
	// The actual implementation will need to match labels to indices
	// This is handled by the test helper below
}

func TestToggleLoadModel(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	if m.loadModelIndex != 0 {
		t.Errorf("expected default loadModelIndex=0, got %d", m.loadModelIndex)
	}

	// Toggle to open-loop
	m.loadModelIndex = 1
	if m.loadModelIndex != 1 {
		t.Errorf("expected loadModelIndex=1 after toggle, got %d", m.loadModelIndex)
	}
}
```

Note: The `setInputValues` helper needs to match labels to indices. This is a challenge because field indices shift based on loadModelIndex. The actual implementation will rebuild fieldDefs when loadModelIndex changes, so the test helper needs to work with the rebuilt state.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestBuildFieldDefs -v -count=1 2>&1 | head -20`
Expected: FAIL — `loadModelIndex` undefined

- [ ] **Step 3: Implement config screen changes**

Modify `internal/tui/config_screen.go`. The key changes:

1. Add `fieldType` to `fieldDef`
2. Add `loadModelIndex` to `configModel`
3. Rebuild fieldDefs when loadModelIndex changes (for completion mode only)
4. Handle `←`/`→` keys on the toggle field
5. Update `validate()` to handle open-loop fields
6. Update `view()` to render the toggle

Here's the full updated file:

```go
package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"embedding_benchmark/internal/bench"
)

type fieldDef struct {
	label       string
	placeholder string
	defaultVal  string
	password    bool
	fieldType   string // "" (textinput, default) or "toggle"
}

type configModel struct {
	apiMode        string
	testMode       string
	inputs         []textinput.Model
	fieldDefs      []fieldDef
	focusIndex     int
	err            string
	width          int
	loadModelIndex int // 0 = closed-loop, 1 = open-loop (completion only)
}

func newConfigModel(apiMode, testMode string) configModel {
	m := configModel{
		apiMode:  apiMode,
		testMode: testMode,
	}
	m.rebuildFields()
	if len(m.inputs) > 0 {
		m.inputs[0].Focus()
	}
	return m
}

// rebuildFields constructs fieldDefs and inputs based on current apiMode/testMode/loadModelIndex.
func (m *configModel) rebuildFields() {
	m.fieldDefs = buildFieldDefs(m.apiMode, m.testMode, m.loadModelIndex)
	m.inputs = make([]textinput.Model, len(m.fieldDefs))
	for i, fd := range m.fieldDefs {
		t := textinput.New()
		t.Placeholder = fd.placeholder
		t.SetValue(fd.defaultVal)
		t.CharLimit = 512
		if fd.password {
			t.EchoMode = textinput.EchoPassword
		}
		m.inputs[i] = t
	}
	// Clamp focusIndex
	if m.focusIndex >= len(m.inputs) {
		m.focusIndex = len(m.inputs) - 1
	}
	if m.focusIndex < 0 {
		m.focusIndex = 0
	}
}

func buildFieldDefs(apiMode, testMode string, loadModelIndex int) []fieldDef {
	isCompletion := apiMode == "completion"
	isAnthropicMsg := apiMode == "anthropic_messages"
	isCompletionLike := isCompletion || isAnthropicMsg
	isPK := testMode == "pk"
	isOpenLoop := isCompletion && loadModelIndex == 1

	customParamsPlaceholder := `optional JSON, e.g. {"temperature":0.7}`

	maxTokPlaceholder := "e.g. 256  (0 = unlimited)"
	if isAnthropicMsg {
		maxTokPlaceholder = "e.g. 4096  (0 → defaults to 4096)"
	}

	// Load Model toggle field (only for completion mode)
	loadModelField := fieldDef{
		label:     "Load Model",
		fieldType: "toggle",
	}

	// Concurrency fields for closed-loop vs open-loop
	concurrencyField := fieldDef{label: "Concurrency", placeholder: "e.g. 10"}
	maxInFlightField := fieldDef{label: "Max In-Flight", placeholder: "equivalent to Concurrency, e.g. 10"}
	requestRateField := fieldDef{label: "Request Rate", placeholder: "requests per second, e.g. 10"}

	if isPK {
		urlPlaceholderA := "https://api.openai.com/v1/embeddings"
		modelPlaceholderA := "e.g. text-embedding-3-small"
		urlPlaceholderB := "https://..."
		modelPlaceholderB := "e.g. text-embedding-ada-002"
		keyPlaceholder := "sk-..."
		if isCompletion {
			urlPlaceholderA = "https://api.openai.com/v1/chat/completions"
			modelPlaceholderA = "e.g. gpt-4o-mini"
			urlPlaceholderB = "https://..."
			modelPlaceholderB = "e.g. gpt-4o-mini"
		} else if isAnthropicMsg {
			urlPlaceholderA = "https://api.anthropic.com/v1/messages"
			modelPlaceholderA = "e.g. claude-sonnet-4-6"
			urlPlaceholderB = "https://api.anthropic.com/v1/messages"
			modelPlaceholderB = "e.g. claude-haiku-4-5"
			keyPlaceholder = "sk-ant-..."
		}
		defs := []fieldDef{
			{label: "Provider A Name", placeholder: "e.g. Provider A"},
			{label: "Provider A URL", placeholder: urlPlaceholderA},
			{label: "Provider A API Key", placeholder: keyPlaceholder, password: true},
			{label: "Provider A Model", placeholder: modelPlaceholderA},
			{label: "Custom Params", placeholder: customParamsPlaceholder},
			{label: "Provider B Name", placeholder: "e.g. Provider B"},
			{label: "Provider B URL", placeholder: urlPlaceholderB},
			{label: "Provider B API Key", placeholder: keyPlaceholder, password: true},
			{label: "Provider B Model", placeholder: modelPlaceholderB},
			{label: "Custom Params", placeholder: customParamsPlaceholder},
		}
		// Add Load Model toggle for completion mode
		if isCompletion {
			defs = append(defs, loadModelField)
		}
		// Add concurrency fields based on load model
		if isOpenLoop {
			defs = append(defs, maxInFlightField, requestRateField)
		} else {
			defs = append(defs, concurrencyField)
		}
		defs = append(defs,
			fieldDef{label: "Total Requests", placeholder: "e.g. 100"},
			fieldDef{label: "Input Tokens", placeholder: "e.g. 500"},
		)
		if isCompletionLike {
			defs = append(defs,
				fieldDef{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
				fieldDef{label: "System Prompt", placeholder: "optional, leave blank to skip"},
			)
		}
		return defs
	}

	// Single provider
	urlPlaceholder := "https://api.openai.com/v1/embeddings"
	modelPlaceholder := "e.g. text-embedding-3-small"
	keyPlaceholder := "sk-..."
	if isCompletion {
		urlPlaceholder = "https://api.openai.com/v1/chat/completions"
		modelPlaceholder = "e.g. gpt-4o-mini"
	} else if isAnthropicMsg {
		urlPlaceholder = "https://api.anthropic.com/v1/messages"
		modelPlaceholder = "e.g. claude-sonnet-4-6"
		keyPlaceholder = "sk-ant-..."
	}
	defs := []fieldDef{
		{label: "API URL", placeholder: urlPlaceholder},
		{label: "API Key", placeholder: keyPlaceholder, password: true},
		{label: "Model", placeholder: modelPlaceholder},
		{label: "Custom Params", placeholder: customParamsPlaceholder},
	}
	// Add Load Model toggle for completion mode
	if isCompletion {
		defs = append(defs, loadModelField)
	}
	// Add concurrency fields based on load model
	if isOpenLoop {
		defs = append(defs, maxInFlightField, requestRateField)
	} else {
		defs = append(defs, concurrencyField)
	}
	defs = append(defs,
		fieldDef{label: "Total Requests", placeholder: "e.g. 100"},
		fieldDef{label: "Input Tokens", placeholder: "e.g. 500"},
	)
	if isCompletionLike {
		defs = append(defs,
			fieldDef{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
			fieldDef{label: "System Prompt", placeholder: "optional, leave blank to skip"},
		)
	}
	return defs
}

func (m *configModel) setWidth(w int) {
	m.width = w
}

// isToggleField returns true if the current focused field is the Load Model toggle.
func (m configModel) isToggleField() bool {
	if m.focusIndex < 0 || m.focusIndex >= len(m.fieldDefs) {
		return false
	}
	return m.fieldDefs[m.focusIndex].fieldType == "toggle"
}

// toggleLoadModel switches between closed-loop and open-loop, rebuilding fields.
func (m *configModel) toggleLoadModel() {
	if m.apiMode != bench.ModeCompletion {
		return // only completion mode supports open-loop
	}
	m.loadModelIndex = 1 - m.loadModelIndex // toggle 0↔1
	m.rebuildFields()
}

func (m configModel) update(msg tea.Msg) (configModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.inputs[m.focusIndex].Blur()
			m.focusIndex = (m.focusIndex + 1) % len(m.inputs)
			return m, m.inputs[m.focusIndex].Focus()
		case "shift+tab":
			m.inputs[m.focusIndex].Blur()
			m.focusIndex = (m.focusIndex - 1 + len(m.inputs)) % len(m.inputs)
			return m, m.inputs[m.focusIndex].Focus()
		case "left", "right":
			if m.isToggleField() {
				m.toggleLoadModel()
				return m, nil
			}
		}
	}

	// Delegate to focused input (toggle fields have no textinput to update)
	if !m.isToggleField() {
		var cmd tea.Cmd
		m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
		return m, cmd
	}
	return m, nil
}

// validate returns (providers, cfg, err) if valid.
func (m configModel) validate() ([]bench.ProviderConfig, bench.BenchConfig, error) {
	vals := make([]string, len(m.inputs))
	for i, inp := range m.inputs {
		vals[i] = strings.TrimSpace(inp.Value())
	}

	isPK := m.testMode == "pk"
	isCompletion := m.apiMode == "completion"
	isAnthropicMsg := m.apiMode == "anthropic_messages"
	isCompletionLike := isCompletion || isAnthropicMsg
	isOpenLoop := isCompletion && m.loadModelIndex == 1

	parseInt := func(s, name string) (int, error) {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", name)
		}
		return v, nil
	}

	// Helper to find field index by label
	fieldIdx := func(label string) int {
		for i, fd := range m.fieldDefs {
			if fd.label == label {
				return i
			}
		}
		return -1
	}

	if isPK {
		// Find indices by label since they shift based on load model
		urlAIdx := fieldIdx("Provider A URL")
		modelAIdx := fieldIdx("Provider A Model")
		customAIdx := fieldIdx("Custom Params") // first Custom Params
		urlBIdx := fieldIdx("Provider B URL")
		modelBIdx := fieldIdx("Provider B Model")
		// Second Custom Params is after Provider B Model
		customBIdx := -1
		for i, fd := range m.fieldDefs {
			if fd.label == "Custom Params" && i > customAIdx {
				customBIdx = i
				break
			}
		}

		urlA, err := bench.NormalizeURL(vals[urlAIdx])
		if err != nil {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider A URL: %w", err)
		}
		if vals[modelAIdx] == "" {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider A Model is required")
		}
		if _, err := validateCustomParams(vals[customAIdx], "Provider A"); err != nil {
			return nil, bench.BenchConfig{}, err
		}
		urlB, err := bench.NormalizeURL(vals[urlBIdx])
		if err != nil {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider B URL: %w", err)
		}
		if vals[modelBIdx] == "" {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider B Model is required")
		}
		if _, err := validateCustomParams(vals[customBIdx], "Provider B"); err != nil {
			return nil, bench.BenchConfig{}, err
		}

		var c, rate int
		if isOpenLoop {
			mifIdx := fieldIdx("Max In-Flight")
			c, err = parseInt(vals[mifIdx], "Max In-Flight")
			if err != nil {
				return nil, bench.BenchConfig{}, err
			}
			rateIdx := fieldIdx("Request Rate")
			rate, err = parseInt(vals[rateIdx], "Request Rate")
			if err != nil {
				return nil, bench.BenchConfig{}, err
			}
		} else {
			concIdx := fieldIdx("Concurrency")
			c, err = parseInt(vals[concIdx], "Concurrency")
			if err != nil {
				return nil, bench.BenchConfig{}, err
			}
		}

		nIdx := fieldIdx("Total Requests")
		n, err := parseInt(vals[nIdx], "Total Requests")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
		tokensIdx := fieldIdx("Input Tokens")
		tokens, err := parseInt(vals[tokensIdx], "Input Tokens")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}

		mode := bench.ModeEmbedding
		if isCompletion {
			mode = bench.ModeCompletion
		}
		if isAnthropicMsg {
			mode = bench.ModeAnthropicMessages
		}
		cfg := bench.BenchConfig{
			Mode:          mode,
			Concurrency:   c,
			TotalRequests: n,
			TargetTokens:  tokens,
		}
		if isOpenLoop {
			cfg.LoadModel = bench.LoadModelOpenLoop
			cfg.MaxInFlight = c
			cfg.RequestRate = rate
		} else {
			cfg.LoadModel = bench.LoadModelClosedLoop
		}
		if isCompletionLike {
			maxTokIdx := fieldIdx("Max Output Tokens")
			sysIdx := fieldIdx("System Prompt")
			maxTok, _ := strconv.Atoi(vals[maxTokIdx])
			cfg.MaxOutputTokens = maxTok
			cfg.SystemPrompt = vals[sysIdx]
		}
		providers := []bench.ProviderConfig{
			{Name: vals[fieldIdx("Provider A Name")], URL: urlA, APIKey: vals[fieldIdx("Provider A API Key")], Model: vals[modelAIdx], CustomParams: vals[customAIdx]},
			{Name: vals[fieldIdx("Provider B Name")], URL: urlB, APIKey: vals[fieldIdx("Provider B API Key")], Model: vals[modelBIdx], CustomParams: vals[customBIdx]},
		}
		return providers, cfg, nil
	}

	// Single provider — find indices by label
	urlIdx := fieldIdx("API URL")
	keyIdx := fieldIdx("API Key")
	modelIdx := fieldIdx("Model")
	customIdx := fieldIdx("Custom Params")

	apiURL, err := bench.NormalizeURL(vals[urlIdx])
	if err != nil {
		return nil, bench.BenchConfig{}, fmt.Errorf("API URL: %w", err)
	}
	if vals[modelIdx] == "" {
		return nil, bench.BenchConfig{}, fmt.Errorf("Model is required")
	}
	if _, err := validateCustomParams(vals[customIdx], "Custom Params"); err != nil {
		return nil, bench.BenchConfig{}, err
	}

	var c, rate int
	if isOpenLoop {
		mifIdx := fieldIdx("Max In-Flight")
		c, err = parseInt(vals[mifIdx], "Max In-Flight")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
		rateIdx := fieldIdx("Request Rate")
		rate, err = parseInt(vals[rateIdx], "Request Rate")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
	} else {
		concIdx := fieldIdx("Concurrency")
		c, err = parseInt(vals[concIdx], "Concurrency")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
	}

	nIdx := fieldIdx("Total Requests")
	n, err := parseInt(vals[nIdx], "Total Requests")
	if err != nil {
		return nil, bench.BenchConfig{}, err
	}
	tokensIdx := fieldIdx("Input Tokens")
	tokens, err := parseInt(vals[tokensIdx], "Input Tokens")
	if err != nil {
		return nil, bench.BenchConfig{}, err
	}

	mode := bench.ModeEmbedding
	if isCompletion {
		mode = bench.ModeCompletion
	}
	if isAnthropicMsg {
		mode = bench.ModeAnthropicMessages
	}
	cfg := bench.BenchConfig{
		Mode:          mode,
		Concurrency:   c,
		TotalRequests: n,
		TargetTokens:  tokens,
	}
	if isOpenLoop {
		cfg.LoadModel = bench.LoadModelOpenLoop
		cfg.MaxInFlight = c
		cfg.RequestRate = rate
	} else {
		cfg.LoadModel = bench.LoadModelClosedLoop
	}
	if isCompletionLike {
		maxTokIdx := fieldIdx("Max Output Tokens")
		sysIdx := fieldIdx("System Prompt")
		maxTok, _ := strconv.Atoi(vals[maxTokIdx])
		cfg.MaxOutputTokens = maxTok
		cfg.SystemPrompt = vals[sysIdx]
	}
	providers := []bench.ProviderConfig{
		{Name: "Provider", URL: apiURL, APIKey: vals[keyIdx], Model: vals[modelIdx], CustomParams: vals[customIdx]},
	}
	return providers, cfg, nil
}

func (m configModel) view(width, height int) string {
	var sb strings.Builder

	apiLabel := "Embedding"
	switch m.apiMode {
	case "completion":
		apiLabel = "Chat Completion"
	case "anthropic_messages":
		apiLabel = "Anthropic Messages"
	}
	testLabel := "Single Provider"
	if m.testMode == "pk" {
		testLabel = "PK Mode"
	}
	sb.WriteString(titleStyle.Render("Configure Benchmark"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render(fmt.Sprintf("%s  •  %s", apiLabel, testLabel)))
	sb.WriteString("\n\n")

	isPK := m.testMode == "pk"
	i := 0
	renderSection := func(heading string, count int) {
		if heading != "" {
			sb.WriteString(sectionStyle.Render(heading))
			sb.WriteString("\n")
		}
		for end := i + count; i < end && i < len(m.inputs); i++ {
			label := labelStyle.Render(m.fieldDefs[i].label + ":")
			focused := i == m.focusIndex
			cursor := "  "
			if focused {
				cursor = "> "
			}
			if m.fieldDefs[i].fieldType == "toggle" {
				// Render radio button toggle
				toggle := renderLoadModelToggle(m.loadModelIndex)
				sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, toggle))
			} else {
				inp := m.inputs[i].View()
				sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, inp))
			}
		}
	}

	if isPK {
		renderSection("-- Provider A --", 5)
		renderSection("-- Provider B --", 5)
		sharedCount := len(m.fieldDefs) - 10
		renderSection("-- Shared Parameters --", sharedCount)
	} else {
		renderSection("", len(m.fieldDefs))
	}

	if m.err != "" {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render("Error: " + m.err))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	help := "tab/shift+tab navigate  •  ctrl+s start benchmark  •  esc back  •  ctrl+c quit"
	if m.isToggleField() {
		help = "←/→ toggle load model  •  " + help
	}
	sb.WriteString(helpStyle.Render(help))
	return sb.String()
}

// renderLoadModelToggle renders the radio button for load model selection.
func renderLoadModelToggle(loadModelIndex int) string {
	if loadModelIndex == 0 {
		return "(●) closed-loop  ( ) open-loop"
	}
	return "( ) closed-loop  (●) open-loop"
}
```

- [ ] **Step 4: Update test helper to work with dynamic fields**

The test helper `setInputValues` needs to be rewritten to work with the new field-def-based approach. Update `internal/tui/config_screen_test.go`:

```go
package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"

	"embedding_benchmark/internal/bench"
)

// setInputsByLabel sets input values by matching field labels.
func setInputsByLabel(m *configModel, values map[string]string) {
	for i, fd := range m.fieldDefs {
		if val, ok := values[fd.label]; ok {
			m.inputs[i] = setTextInputValue(m.inputs[i], val)
		}
	}
}

func setTextInputValue(ti textinput.Model, val string) textinput.Model {
	ti.SetValue(val)
	return ti
}

func TestBuildFieldDefs_CompletionClosedLoop(t *testing.T) {
	defs := buildFieldDefs(bench.ModeCompletion, "single", 0)
	labels := fieldLabels(defs)

	if !containsLabel(labels, "Concurrency") {
		t.Error("closed-loop should include Concurrency field")
	}
	if containsLabel(labels, "Max In-Flight") {
		t.Error("closed-loop should not include Max In-Flight field")
	}
	if containsLabel(labels, "Request Rate") {
		t.Error("closed-loop should not include Request Rate field")
	}
	if containsLabel(labels, "Load Model") {
		t.Error("Load Model toggle should be present in completion mode")
	}
}

func TestBuildFieldDefs_CompletionOpenLoop(t *testing.T) {
	defs := buildFieldDefs(bench.ModeCompletion, "single", 1)
	labels := fieldLabels(defs)

	if containsLabel(labels, "Concurrency") {
		t.Error("open-loop should not include Concurrency field")
	}
	if !containsLabel(labels, "Max In-Flight") {
		t.Error("open-loop should include Max In-Flight field")
	}
	if !containsLabel(labels, "Request Rate") {
		t.Error("open-loop should include Request Rate field")
	}
}

func TestBuildFieldDefs_Embedding(t *testing.T) {
	defs := buildFieldDefs(bench.ModeEmbedding, "single", 0)
	labels := fieldLabels(defs)

	if !containsLabel(labels, "Concurrency") {
		t.Error("embedding should include Concurrency")
	}
	if containsLabel(labels, "Load Model") {
		t.Error("embedding should not include Load Model toggle")
	}
}

func TestBuildFieldDefs_AnthropicMessages(t *testing.T) {
	defs := buildFieldDefs(bench.ModeAnthropicMessages, "single", 0)
	labels := fieldLabels(defs)

	if !containsLabel(labels, "Concurrency") {
		t.Error("anthropic should include Concurrency")
	}
	if containsLabel(labels, "Load Model") {
		t.Error("anthropic should not include Load Model toggle")
	}
}

func TestValidate_OpenLoopRequiresMaxInFlight(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1
	m.rebuildFields()

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "",
		"Request Rate":   "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, _, err := m.validate()
	if err == nil {
		t.Error("expected error when Max In-Flight is empty in open-loop mode")
	}
	if !strings.Contains(err.Error(), "Max In-Flight") {
		t.Errorf("error should mention Max In-Flight, got: %v", err)
	}
}

func TestValidate_OpenLoopRequiresRequestRate(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1
	m.rebuildFields()

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "10",
		"Request Rate":   "",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, _, err := m.validate()
	if err == nil {
		t.Error("expected error when Request Rate is empty in open-loop mode")
	}
	if !strings.Contains(err.Error(), "Request Rate") {
		t.Errorf("error should mention Request Rate, got: %v", err)
	}
}

func TestValidate_ClosedLoopIgnoresOpenLoopFields(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	// loadModelIndex defaults to 0 (closed-loop)

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Concurrency":    "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("closed-loop should not require open-loop fields, got: %v", err)
	}
	if cfg.LoadModel != bench.LoadModelClosedLoop {
		t.Errorf("expected LoadModel=closed_loop, got %s", cfg.LoadModel)
	}
}

func TestValidate_OpenLoopSetsConfigFields(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1
	m.rebuildFields()

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "5",
		"Request Rate":   "20",
		"Total Requests": "50",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LoadModel != bench.LoadModelOpenLoop {
		t.Errorf("expected LoadModel=open_loop, got %s", cfg.LoadModel)
	}
	if cfg.MaxInFlight != 5 {
		t.Errorf("expected MaxInFlight=5, got %d", cfg.MaxInFlight)
	}
	if cfg.RequestRate != 20 {
		t.Errorf("expected RequestRate=20, got %d", cfg.RequestRate)
	}
	if cfg.TotalRequests != 50 {
		t.Errorf("expected TotalRequests=50, got %d", cfg.TotalRequests)
	}
}

func TestValidate_OpenLoopPKMode(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "pk")
	m.loadModelIndex = 1
	m.rebuildFields()

	setInputsByLabel(&m, map[string]string{
		"Provider A URL":   "https://api.openai.com/v1/chat/completions",
		"Provider A Model": "gpt-4o-mini",
		"Provider B URL":   "https://api.openai.com/v1/chat/completions",
		"Provider B Model": "gpt-4o",
		"Max In-Flight":    "10",
		"Request Rate":     "30",
		"Total Requests":   "20",
		"Input Tokens":     "100",
	})

	providers, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
	if cfg.LoadModel != bench.LoadModelOpenLoop {
		t.Errorf("expected LoadModel=open_loop, got %s", cfg.LoadModel)
	}
	if cfg.MaxInFlight != 10 {
		t.Errorf("expected MaxInFlight=10, got %d", cfg.MaxInFlight)
	}
	if cfg.RequestRate != 30 {
		t.Errorf("expected RequestRate=30, got %d", cfg.RequestRate)
	}
}

func TestToggleLoadModel(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	if m.loadModelIndex != 0 {
		t.Errorf("expected default loadModelIndex=0, got %d", m.loadModelIndex)
	}

	m.toggleLoadModel()
	if m.loadModelIndex != 1 {
		t.Errorf("expected loadModelIndex=1 after toggle, got %d", m.loadModelIndex)
	}

	// Verify fields were rebuilt
	labels := fieldLabels(m.fieldDefs)
	if !containsLabel(labels, "Max In-Flight") {
		t.Error("after toggle to open-loop, should include Max In-Flight")
	}

	m.toggleLoadModel()
	if m.loadModelIndex != 0 {
		t.Errorf("expected loadModelIndex=0 after second toggle, got %d", m.loadModelIndex)
	}
	labels = fieldLabels(m.fieldDefs)
	if !containsLabel(labels, "Concurrency") {
		t.Error("after toggle back to closed-loop, should include Concurrency")
	}
}

func TestToggleLoadModel_IgnoredForEmbedding(t *testing.T) {
	m := newConfigModel(bench.ModeEmbedding, "single")
	m.toggleLoadModel() // should be no-op
	if m.loadModelIndex != 0 {
		t.Errorf("toggle should be no-op for embedding, got loadModelIndex=%d", m.loadModelIndex)
	}
}

func fieldLabels(defs []fieldDef) []string {
	labels := make([]string, len(defs))
	for i, d := range defs {
		labels[i] = d.label
	}
	return labels
}

func containsLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/ -run TestBuildFieldDefs -v -count=1`
Expected: all PASS

Run: `go test ./internal/tui/ -run TestValidate_ -v -count=1`
Expected: all PASS

Run: `go test ./internal/tui/ -run TestToggle -v -count=1`
Expected: all PASS

- [ ] **Step 6: Verify all existing tests still pass**

Run: `go test ./... -count=1 -timeout 60s`
Expected: all PASS

- [ ] **Step 7: Commit**

```bash
git add internal/tui/config_screen.go internal/tui/config_screen_test.go
git commit -m "feat: add Load Model toggle with dynamic field visibility in config screen"
```

---

### Task 5: Update startBench dispatch and results screen

**Files:**
- Modify: `internal/tui/running_screen.go:93-118`
- Modify: `internal/tui/results_screen.go:293-480`

- [ ] **Step 1: Update startBench dispatch**

In `internal/tui/running_screen.go`, modify the dispatch logic in the goroutine:

Replace this block (lines 107-116):
```go
if cfg.Mode == bench.ModeEmbedding {
    r := bench.RunEmbeddingBench(ctx, pv, cfg, testText, actualTokens, onProgress)
    p.Send(BenchDoneMsg{ProviderIndex: idx, EmbeddingReport: &r})
} else if cfg.Mode == bench.ModeAnthropicMessages {
    r := bench.RunAnthropicMessagesBench(ctx, pv, cfg, testText, actualTokens, tkm, onProgress)
    p.Send(BenchDoneMsg{ProviderIndex: idx, CompletionReport: &r})
} else {
    r := bench.RunCompletionBench(ctx, pv, cfg, testText, actualTokens, tkm, onProgress)
    p.Send(BenchDoneMsg{ProviderIndex: idx, CompletionReport: &r})
}
```

With:
```go
if cfg.Mode == bench.ModeEmbedding {
    r := bench.RunEmbeddingBench(ctx, pv, cfg, testText, actualTokens, onProgress)
    p.Send(BenchDoneMsg{ProviderIndex: idx, EmbeddingReport: &r})
} else if cfg.Mode == bench.ModeAnthropicMessages {
    r := bench.RunAnthropicMessagesBench(ctx, pv, cfg, testText, actualTokens, tkm, onProgress)
    p.Send(BenchDoneMsg{ProviderIndex: idx, CompletionReport: &r})
} else if cfg.LoadModel == bench.LoadModelOpenLoop {
    r := bench.RunOpenLoopCompletionBench(ctx, pv, cfg, testText, actualTokens, tkm, onProgress)
    p.Send(BenchDoneMsg{ProviderIndex: idx, CompletionReport: &r})
} else {
    r := bench.RunCompletionBench(ctx, pv, cfg, testText, actualTokens, tkm, onProgress)
    p.Send(BenchDoneMsg{ProviderIndex: idx, CompletionReport: &r})
}
```

- [ ] **Step 2: Add Queue Time rows to results screen (single provider)**

In `internal/tui/results_screen.go`, modify `renderCompletionSingle` to add Queue Time section after E2E rows.

After the E2E rows block (around line 361), add:

```go
	// Queue Time section (open-loop only)
	if r.QueueTimeAvg > 0 || r.QueueTimeP50 > 0 || r.QueueTimeP90 > 0 || r.QueueTimeP99 > 0 {
		sb.WriteString(dimStyle.Render("  " + strings.Repeat("─", 36)))
		sb.WriteString("\n")
		renderTwoColTable(sb, [][]string{
			{"Queue Time Avg", fmtMs(r.QueueTimeAvg)},
			{"Queue Time P50", fmtMs(r.QueueTimeP50)},
			{"Queue Time P90", fmtMs(r.QueueTimeP90)},
			{"Queue Time P99", fmtMs(r.QueueTimeP99)},
		})
	}
```

Insert this right after the `renderTwoColTable(sb, e2eRows)` call and before the error count block.

- [ ] **Step 3: Add Queue Time rows to results screen (PK mode)**

In `internal/tui/results_screen.go`, modify `renderCompletionPK`.

Add Queue Time rows to the `compRows` slice, after the E2E rows and before the closing `}`:

```go
			{"E2E P99", a.E2Ep99, b.E2Ep99, fmtMs, false, false},
			{"", 0, 0, nil, false, true},
			{"Queue Time Avg", a.QueueTimeAvg, b.QueueTimeAvg, fmtMs, false, false},
			{"Queue Time P50", a.QueueTimeP50, b.QueueTimeP50, fmtMs, false, false},
			{"Queue Time P90", a.QueueTimeP90, b.QueueTimeP90, fmtMs, false, false},
			{"Queue Time P99", a.QueueTimeP99, b.QueueTimeP99, fmtMs, false, false},
		}
```

However, Queue Time rows should only appear when at least one provider has non-zero queue times. Replace the `compRows` construction to conditionally include queue time rows:

```go
	var compRows []row
	if aValid && bValid {
		compRows = []row{
			{"RPS", a.RPS, b.RPS, func(v float64) string { return fmtF(v, 2) }, true, false},
			{"Input TPS", a.InputTPS, b.InputTPS, func(v float64) string { return fmtF(v, 1) }, true, false},
			{"Output TPS", a.OutputTPS, b.OutputTPS, func(v float64) string { return fmtF(v, 1) }, true, false},
			{"Input TPM", a.InputTPM, b.InputTPM, func(v float64) string { return fmtF(v, 0) }, true, false},
			{"Output TPM", a.OutputTPM, b.OutputTPM, func(v float64) string { return fmtF(v, 0) }, true, false},
			{"", 0, 0, nil, false, true},
			{"TTFT Avg", a.TTFTAvg, b.TTFTAvg, fmtMs, false, false},
			{"TTFT P50", a.TTFTp50, b.TTFTp50, fmtMs, false, false},
			{"TTFT P90", a.TTFTp90, b.TTFTp90, fmtMs, false, false},
			{"TTFT P99", a.TTFTp99, b.TTFTp99, fmtMs, false, false},
			{"", 0, 0, nil, false, true},
			{"TPOT Avg", a.TPOTAvg, b.TPOTAvg, fmtMsPerTok, false, false},
			{"TPOT P50", a.TPOTp50, b.TPOTp50, fmtMsPerTok, false, false},
			{"TPOT P90", a.TPOTp90, b.TPOTp90, fmtMsPerTok, false, false},
			{"TPOT P99", a.TPOTp99, b.TPOTp99, fmtMsPerTok, false, false},
			{"", 0, 0, nil, false, true},
			{"E2E Avg", a.E2EAvg, b.E2EAvg, fmtMs, false, false},
			{"E2E P50", a.E2Ep50, b.E2Ep50, fmtMs, false, false},
			{"E2E P90", a.E2Ep90, b.E2Ep90, fmtMs, false, false},
			{"E2E P99", a.E2Ep99, b.E2Ep99, fmtMs, false, false},
		}
		// Add Queue Time rows only if at least one provider has queue time data
		if a.QueueTimeAvg > 0 || b.QueueTimeAvg > 0 || a.QueueTimeP50 > 0 || b.QueueTimeP50 > 0 {
			compRows = append(compRows,
				row{"", 0, 0, nil, false, true},
				row{"Queue Time Avg", a.QueueTimeAvg, b.QueueTimeAvg, fmtMs, false, false},
				row{"Queue Time P50", a.QueueTimeP50, b.QueueTimeP50, fmtMs, false, false},
				row{"Queue Time P90", a.QueueTimeP90, b.QueueTimeP90, fmtMs, false, false},
				row{"Queue Time P99", a.QueueTimeP99, b.QueueTimeP99, fmtMs, false, false},
			)
		}
	}
```

- [ ] **Step 4: Verify build**

Run: `go build ./...`
Expected: no errors

- [ ] **Step 5: Run all tests**

Run: `go test ./... -count=1 -timeout 60s`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/running_screen.go internal/tui/results_screen.go
git commit -m "feat: add open-loop dispatch in startBench and Queue Time display in results"
```

---

### Task 6: Update AGENTS.md

**Files:**
- Modify: `AGENTS.md`

- [ ] **Step 1: Add open-loop documentation**

Add a new section after "Benchmark Modes" in `AGENTS.md`:

```markdown
### Open-Loop Mode (Chat Completion only)

- **Load Model**: Radio toggle in config screen — `closed-loop` (default) or `open-loop`
- **Closed-loop**: Fixed workers drain a task queue. `Concurrency` controls worker count.
- **Open-loop**: Generator goroutine produces requests at Poisson intervals (`Request Rate` req/s). Semaphore limits max in-flight requests (`Max In-Flight`). Queue time is measured and displayed.
- Open-loop fields: `Max In-Flight` (replaces `Concurrency`), `Request Rate` (requests per second)
- PK mode enforces same Load Model for both providers
- New metrics: Queue Time Avg/P50/P90/P99 (only shown for open-loop results)
```

Also update the `BenchConfig` key types section:

```markdown
```go
type BenchConfig struct {
    Mode, Concurrency, TotalRequests, TargetTokens, MaxOutputTokens int;
    SystemPrompt string;
    LoadModel string;        // "closed_loop" or "open_loop"
    RequestRate int;         // open-loop: requests per second
    MaxInFlight int;         // open-loop: max concurrent in-flight
}
```
```

- [ ] **Step 2: Commit**

```bash
git add AGENTS.md
git commit -m "docs: add open-loop mode to AGENTS.md"
```

---

### Task 7: Final integration verification

- [ ] **Step 1: Full build**

Run: `go build -o embedding_benchmark .`
Expected: binary builds successfully

- [ ] **Step 2: All tests pass**

Run: `go test ./... -v -count=1 -timeout 120s`
Expected: all tests PASS

- [ ] **Step 3: Verify no regressions in existing functionality**

Run existing test suites:
```bash
go test ./internal/bench/ -v -count=1 -timeout 60s
go test ./internal/tui/ -v -count=1 -timeout 60s
```
Expected: all PASS
