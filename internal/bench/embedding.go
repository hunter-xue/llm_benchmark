package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResult struct {
	Duration time.Duration
	Tokens   int
	Err      error
}

// RunEmbeddingBench runs a concurrent embedding benchmark and returns the aggregated report.
// onProgress is called after each request with current success and error counts.
func RunEmbeddingBench(
	ctx context.Context,
	provider ProviderConfig,
	cfg BenchConfig,
	testText string,
	actualInputTokens int,
	onProgress func(completed, errors int),
) EmbeddingReport {
	results := make(chan embeddingResult, cfg.TotalRequests)
	taskQueue := make(chan struct{}, cfg.TotalRequests)
	for i := 0; i < cfg.TotalRequests; i++ {
		taskQueue <- struct{}{}
	}
	close(taskQueue)

	var (
		wg       sync.WaitGroup
		succAtomic int64
		errAtomic  int64
	)

	startTime := time.Now()

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 120 * time.Second}

			for range taskQueue {
				res := doEmbeddingRequest(ctx, client, provider, testText, actualInputTokens)
				results <- res
				if res.Err != nil {
					atomic.AddInt64(&errAtomic, 1)
				} else {
					atomic.AddInt64(&succAtomic, 1)
				}
				if onProgress != nil {
					onProgress(int(atomic.LoadInt64(&succAtomic)), int(atomic.LoadInt64(&errAtomic)))
				}
			}
		}()
	}

	wg.Wait()
	close(results)
	wallTime := time.Since(startTime)

	var latencies []float64
	totalTokens := 0
	errors := 0
	errorDetails := make(map[string]int)

	for r := range results {
		if r.Err != nil {
			errors++
			errorDetails[r.Err.Error()]++
		} else {
			latencies = append(latencies, float64(r.Duration.Milliseconds()))
			totalTokens += r.Tokens
		}
	}
	sort.Float64s(latencies)

	successCount := len(latencies)
	report := EmbeddingReport{
		TotalRequests: cfg.TotalRequests,
		SuccessCount:  successCount,
		ErrorCount:    errors,
		WallTime:      wallTime,
		ErrorDetails:  errorDetails,
		Valid:         successCount > 0,
	}
	if successCount > 0 && wallTime.Seconds() > 0 {
		report.RPS = float64(successCount) / wallTime.Seconds()
		report.InputTPS = float64(totalTokens) / wallTime.Seconds()
		report.InputTPM = float64(totalTokens) / wallTime.Minutes()
		report.LatencyAvg = average(latencies)
		report.LatencyP50 = percentile(latencies, 0.50)
		report.LatencyP90 = percentile(latencies, 0.90)
		report.LatencyP99 = percentile(latencies, 0.99)
	}
	return report
}

func doEmbeddingRequest(ctx context.Context, client *http.Client, provider ProviderConfig, testText string, actualInputTokens int) embeddingResult {
	res := embeddingResult{Tokens: actualInputTokens}

	// Check context before issuing request
	select {
	case <-ctx.Done():
		res.Err = ctx.Err()
		return res
	default:
	}

	payload := embeddingRequest{Model: provider.Model, Input: []string{testText}}
	body, _ := json.Marshal(payload)
	body = MergeCustomParams(body, provider.CustomParams)

	req, err := http.NewRequestWithContext(ctx, "POST", provider.URL, bytes.NewBuffer(body))
	if err != nil {
		res.Err = fmt.Errorf("failed to create request: %w", err)
		return res
	}
	req.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.Err = err
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(errBody) > 0 {
			res.Err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(errBody))
		} else {
			res.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return res
	}

	io.Copy(io.Discard, resp.Body)
	res.Duration = time.Since(start)
	return res
}
