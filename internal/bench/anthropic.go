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

const anthropicVersion = "2023-06-01"
const anthropicDefaultMaxTokens = 4096

type anthropicRequest struct {
	Model     string               `json:"model"`
	MaxTokens int                  `json:"max_tokens"`
	System    string               `json:"system,omitempty"`
	Messages  []completionMessage  `json:"messages"`
	Stream    bool                 `json:"stream"`
}

type anthropicChunk struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

func setAnthropicHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", anthropicVersion)
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
}

// RunAnthropicMessagesBench runs a concurrent streaming Anthropic Messages benchmark.
// It reuses CompletionReport since the metrics are identical to completion mode.
func RunAnthropicMessagesBench(
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

				res := doAnthropicRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm)
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

func doAnthropicRequest(
	ctx context.Context,
	client *http.Client,
	provider ProviderConfig,
	cfg BenchConfig,
	testText string,
	actualInputTokens int,
	tkm *tiktoken.Tiktoken,
) completionResult {
	res := completionResult{InputTokens: actualInputTokens}

	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = anthropicDefaultMaxTokens
	}

	payload := anthropicRequest{
		Model:     provider.Model,
		MaxTokens: maxTokens,
		System:    cfg.SystemPrompt,
		Messages:  []completionMessage{{Role: "user", Content: testText}},
		Stream:    true,
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
	req.Header.Set("Accept", "text/event-stream")
	setAnthropicHeaders(req, provider.APIKey)

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
		eventType      string
	)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			if eventType == "message_stop" {
				break
			}
			continue
		}
		if eventType != "content_block_delta" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var chunk anthropicChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			skippedChunks++
			continue
		}
		if chunk.Delta.Type == "text_delta" && chunk.Delta.Text != "" {
			now := time.Now()
			if !gotFirstToken {
				firstTokenTime = now
				gotFirstToken = true
			}
			lastTokenTime = now
			outputBuf.WriteString(chunk.Delta.Text)
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

// DoAnthropicCompareRequest sends a single non-streaming Anthropic Messages request
// and returns the formatted response headers and pretty-printed JSON body.
func DoAnthropicCompareRequest(ctx context.Context, provider ProviderConfig, cfg BenchConfig, userMessage, customParams string) (headers string, body string, err error) {
	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = anthropicDefaultMaxTokens
	}

	payload := anthropicRequest{
		Model:     provider.Model,
		MaxTokens: maxTokens,
		System:    cfg.SystemPrompt,
		Messages:  []completionMessage{{Role: "user", Content: userMessage}},
		Stream:    false,
	}

	reqBody, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "", "", fmt.Errorf("failed to marshal request: %w", marshalErr)
	}
	reqBody = MergeCustomParams(reqBody, customParams)

	req, reqErr := http.NewRequestWithContext(ctx, "POST", provider.URL, bytes.NewBuffer(reqBody))
	if reqErr != nil {
		return "", "", fmt.Errorf("failed to create request: %w", reqErr)
	}
	setAnthropicHeaders(req, provider.APIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, doErr := client.Do(req)
	if doErr != nil {
		return "", "", doErr
	}
	defer resp.Body.Close()

	rawBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", "", fmt.Errorf("failed to read response: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := strings.TrimSpace(string(rawBody))
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
	}

	headers = FormatResponseHeaders(resp.Proto, resp.Status, resp.Header)

	var parsed any
	if jsonErr := json.Unmarshal(rawBody, &parsed); jsonErr != nil {
		return headers, string(rawBody), nil
	}
	pretty, indentErr := json.MarshalIndent(parsed, "", "  ")
	if indentErr != nil {
		return headers, string(rawBody), nil
	}
	return headers, string(pretty), nil
}
