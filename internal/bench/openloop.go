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
//
// Note: the generator acquires the semaphore before launching each request,
// so when all MaxInFlight slots are occupied the generator blocks and the
// effective arrival rate drops below RequestRate. QueueTime measures the
// per-request wait from generation to actual send.
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

	// Single HTTP client shared across all request goroutines.
	client := &http.Client{Timeout: 300 * time.Second}

	// Generator: produce requests at Poisson intervals.
	for i := 0; i < cfg.TotalRequests; i++ {
		if i > 0 {
			interval := poissonInterval(float64(cfg.RequestRate))
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				// Stop generating new requests. Requests that were never
				// sent are dropped (not recorded as errors), so a cancelled
				// run reports fewer than TotalRequests results.
				goto waitForInFlight
			case <-timer.C:
			}
		}

		generatedAt := time.Now()

		// Acquire semaphore slot (blocks when full).
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

			res := doCompletionRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm, "", nil)
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

	// Aggregate results (same logic as RunCompletionBench).
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
