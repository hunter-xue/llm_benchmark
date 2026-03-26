package bench

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

type completionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionRequest struct {
	Model     string              `json:"model"`
	Messages  []completionMessage `json:"messages"`
	Stream    bool                `json:"stream"`
	MaxTokens int                 `json:"max_tokens,omitempty"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type completionResult struct {
	TTFT          time.Duration
	E2E           time.Duration
	InputTokens   int
	OutputTokens  int
	SkippedChunks int
	Err           error
}

// RunCompletionBench runs a concurrent streaming completion benchmark and returns the report.
func RunCompletionBench(
	ctx context.Context,
	provider ProviderConfig,
	cfg BenchConfig,
	testText string,
	actualInputTokens int,
	tkm *tiktoken.Tiktoken,
	onProgress func(completed, errors int),
) CompletionReport {
	results := make(chan completionResult, cfg.TotalRequests)

	taskQueue := make(chan struct{}, cfg.TotalRequests)
	for i := 0; i < cfg.TotalRequests; i++ {
		taskQueue <- struct{}{}
	}
	close(taskQueue)

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		completed int
		errCount  int
	)

	startTime := time.Now()

	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 300 * time.Second}

			for range taskQueue {
				select {
				case <-ctx.Done():
					results <- completionResult{Err: ctx.Err()}
					mu.Lock()
					errCount++
					c, e := completed, errCount
					mu.Unlock()
					if onProgress != nil {
						onProgress(c, e)
					}
					continue
				default:
				}

				res := doCompletionRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm)
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
					onProgress(c, e)
				}
			}
		}()
	}

	wg.Wait()
	close(results)
	wallTime := time.Since(startTime)

	var (
		ttfts              []float64
		e2es               []float64
		tpots              []float64
		totalInToks        int
		totalOutToks       int
		totalSkippedChunks int
		errors             int
		errorDetails       = make(map[string]int)
	)

	for r := range results {
		if r.Err != nil {
			errors++
			errorDetails[r.Err.Error()]++
		} else {
			ttfts = append(ttfts, float64(r.TTFT.Milliseconds()))
			e2es = append(e2es, float64(r.E2E.Milliseconds()))
			totalInToks += r.InputTokens
			totalOutToks += r.OutputTokens
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

	successCount := len(e2es)
	report := CompletionReport{
		TotalRequests: cfg.TotalRequests,
		SuccessCount:  successCount,
		ErrorCount:    errors,
		WallTime:      wallTime,
		SkippedChunks: totalSkippedChunks,
		ErrorDetails:  errorDetails,
		Valid:         successCount > 0,
	}

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
	}
	return report
}

func doCompletionRequest(
	ctx context.Context,
	client *http.Client,
	provider ProviderConfig,
	cfg BenchConfig,
	testText string,
	actualInputTokens int,
	tkm *tiktoken.Tiktoken,
) completionResult {
	res := completionResult{InputTokens: actualInputTokens}

	messages := []completionMessage{}
	if cfg.SystemPrompt != "" {
		messages = append(messages, completionMessage{Role: "system", Content: cfg.SystemPrompt})
	}
	messages = append(messages, completionMessage{Role: "user", Content: testText})

	payload := completionRequest{
		Model:    provider.Model,
		Messages: messages,
		Stream:   true,
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

	sendTime := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.Err = err
		return res
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if len(errBody) > 0 {
			res.Err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
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
		skippedChunks  int
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Text()
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

		for _, choice := range chunk.Choices {
			content := choice.Delta.Content
			if content == "" {
				continue
			}
			now := time.Now()
			if !gotFirstToken {
				firstTokenTime = now
				gotFirstToken = true
			}
			lastTokenTime = now
			outputBuf.WriteString(content)
		}
	}

	if err := scanner.Err(); err != nil {
		res.Err = fmt.Errorf("failed to read stream: %w", err)
		return res
	}

	if !gotFirstToken {
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

// DoCompareRequest sends a single non-streaming completion request and returns
// the raw response body pretty-printed as JSON. Used for response quality comparison.
// customParams is an optional JSON object string whose key-value pairs are merged
// into the request body, allowing provider-specific or non-standard parameters.
func DoCompareRequest(ctx context.Context, provider ProviderConfig, cfg BenchConfig, testText string, customParams string) (string, error) {
	messages := []completionMessage{}
	if cfg.SystemPrompt != "" {
		messages = append(messages, completionMessage{Role: "system", Content: cfg.SystemPrompt})
	}
	messages = append(messages, completionMessage{Role: "user", Content: testText})

	payload := completionRequest{
		Model:    provider.Model,
		Messages: messages,
		Stream:   false,
	}
	if cfg.MaxOutputTokens > 0 {
		payload.MaxTokens = cfg.MaxOutputTokens
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	body = MergeCustomParams(body, customParams)

	req, err := http.NewRequestWithContext(ctx, "POST", provider.URL, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := strings.TrimSpace(string(rawBody))
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}

	// Pretty-print JSON for readability
	var parsed any
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		// Not valid JSON — return raw body as-is
		return string(rawBody), nil
	}
	pretty, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return string(rawBody), nil
	}
	return string(pretty), nil
}
