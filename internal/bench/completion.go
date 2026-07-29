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
	"sync/atomic"
	"time"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

type completionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completionRequest struct {
	Model         string                `json:"model"`
	Messages      []completionMessage   `json:"messages"`
	Stream        bool                  `json:"stream"`
	MaxTokens     int                   `json:"max_tokens,omitempty"`
	StreamOptions *streamOptionsRequest `json:"stream_options,omitempty"`
}

type streamOptionsRequest struct {
	IncludeUsage bool `json:"include_usage"`
}

type apiUsage struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	TotalTokens      *int `json:"total_tokens"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *apiUsage `json:"usage"`
}

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

// RunCompletionBench runs a concurrent streaming completion benchmark and returns the report.
func RunCompletionBench(
	ctx context.Context,
	provider ProviderConfig,
	cfg BenchConfig,
	testText string,
	actualInputTokens int,
	tkm *tiktoken.Tiktoken,
	onProgress func(ProgressUpdate),
) CompletionReport {
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
						onProgress(ProgressUpdate{
							Completed:     c,
							Errors:        e,
							ErrorDetail:   ctx.Err().Error(),
							ErrorCategory: ClassifyError(ctx.Err()),
						})
					}
					continue
				default:
				}

				reqID := ""
				if logger != nil {
					reqID = fmt.Sprintf("%s-%06d", runID, reqSeq.Add(1))
				}
				res := doCompletionRequest(ctx, client, provider, cfg, testText, actualInputTokens, tkm, reqID, logger)
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
			}
		}()
	}

	wg.Wait()
	close(results)
	if logger != nil {
		logger.Close()
	}
	wallTime := time.Since(startTime)

	var (
		ttfts              []float64
		e2es               []float64
		tpots              []float64
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
	}
	if logger != nil {
		report.LogRequestsFile = logger.RequestsFile()
		report.LogResponsesFile = logger.ResponsesFile()
		report.LogDroppedCount = int(logger.DroppedCount())
		if err := logger.Err(); err != nil {
			report.LogError = err.Error()
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

func applyAPIUsage(res *completionResult, usage *apiUsage) {
	if usage == nil {
		return
	}
	if usage.PromptTokens != nil {
		res.APIPrompt = *usage.PromptTokens
		res.HasAPIUsage = true
	}
	if usage.CompletionTokens != nil {
		res.APICompletion = *usage.CompletionTokens
		res.HasAPIUsage = true
	}
	if usage.TotalTokens != nil {
		res.APITotal = *usage.TotalTokens
		res.HasAPIUsage = true
	}
	if res.HasAPIUsage && usage.TotalTokens == nil {
		res.APITotal = res.APIPrompt + res.APICompletion
	}
}

// DoCompareRequest sends a single non-streaming completion request and returns
// the formatted response headers and pretty-printed JSON body.
// customParams is an optional JSON object string whose key-value pairs are merged
// into the request body, allowing provider-specific or non-standard parameters.
func DoCompareRequest(ctx context.Context, provider ProviderConfig, cfg BenchConfig, testText string, customParams string) (headers string, body string, err error) {
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

	reqBody, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "", "", fmt.Errorf("failed to marshal request: %w", marshalErr)
	}

	reqBody = MergeCustomParams(reqBody, customParams)

	req, reqErr := http.NewRequestWithContext(ctx, "POST", provider.URL, bytes.NewBuffer(reqBody))
	if reqErr != nil {
		return "", "", fmt.Errorf("failed to create request: %w", reqErr)
	}
	req.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

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

	// Pretty-print JSON for readability
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
