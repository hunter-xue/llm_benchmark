package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type CacheHitConfig struct {
	APIMode         string // ModeCompletion or ModeAnthropicMessages
	TestCount       int
	MaxOutputTokens int
	SystemPrompt    string
	Interval        time.Duration
}

type CacheHitResult struct {
	Index               int
	Latency             time.Duration
	PromptTokens        int // OpenAI prompt_tokens / Anthropic input_tokens
	CompletionTokens    int
	TotalTokens         int
	CachedTokens        int // OpenAI cached_tokens / Anthropic cache_read_input_tokens
	CacheCreationTokens int // Anthropic cache_creation_input_tokens only
	HasCachedTokens     bool
	HasCacheCreation    bool
	Err                 error
}

type CacheHitReport struct {
	APIMode                   string
	TotalRequests             int
	SuccessCount              int
	ErrorCount                int
	WallTime                  time.Duration
	Results                   []CacheHitResult
	TotalPromptTokens         int
	TotalCachedTokens         int
	TotalCacheCreationTokens  int
	CacheHitRate              float64
	AvgRequestHitRate         float64
	AvgLatencyMs              float64
	MissingUsageCount         int
	MissingCachedCount        int
	ErrorDetails              map[string]int
	ErrorCategories           map[string]int
	Valid                     bool
}

type cacheHitUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptDetails    *struct {
		CachedTokens *int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type cacheHitResponse struct {
	Usage *cacheHitUsage `json:"usage"`
}

type anthropicCacheHitUsage struct {
	InputTokens              *int `json:"input_tokens"`
	OutputTokens             *int `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
}

type anthropicCacheHitResponse struct {
	Usage *anthropicCacheHitUsage `json:"usage"`
}

// RunCacheHitTest sends repeated non-streaming requests and extracts prompt
// cache hits. Chat Completions reads usage.prompt_tokens_details.cached_tokens;
// Anthropic Messages reads cache_read_input_tokens / cache_creation_input_tokens.
func RunCacheHitTest(
	ctx context.Context,
	provider ProviderConfig,
	cfg CacheHitConfig,
	userPrompt string,
	onProgress func(ProgressUpdate),
) CacheHitReport {
	if cfg.Interval <= 0 {
		cfg.Interval = 3 * time.Second
	}
	if cfg.APIMode == "" {
		cfg.APIMode = ModeCompletion
	}

	report := CacheHitReport{
		APIMode:         cfg.APIMode,
		TotalRequests:   cfg.TestCount,
		Results:         make([]CacheHitResult, 0, cfg.TestCount),
		ErrorDetails:    make(map[string]int),
		ErrorCategories: make(map[string]int),
	}
	client := &http.Client{Timeout: 120 * time.Second}
	start := time.Now()

	for i := 0; i < cfg.TestCount; i++ {
		if i > 0 {
			timer := time.NewTimer(cfg.Interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				result := CacheHitResult{Index: i + 1, Err: ctx.Err()}
				report.Results = append(report.Results, result)
				report.ErrorCount++
				recordError(report.ErrorDetails, report.ErrorCategories, ctx.Err())
				if onProgress != nil {
					onProgress(ProgressUpdate{
						Completed:     report.SuccessCount,
						Errors:        report.ErrorCount,
						ErrorDetail:   ctx.Err().Error(),
						ErrorCategory: ClassifyError(ctx.Err()),
					})
				}
				report.WallTime = time.Since(start)
				return finalizeCacheHitReport(report)
			case <-timer.C:
			}
		}

		var result CacheHitResult
		if cfg.APIMode == ModeAnthropicMessages {
			result = doAnthropicCacheHitRequest(ctx, client, provider, cfg, userPrompt, i+1)
		} else {
			result = doCacheHitRequest(ctx, client, provider, cfg, userPrompt, i+1)
		}
		report.Results = append(report.Results, result)
		if result.Err != nil {
			report.ErrorCount++
			recordError(report.ErrorDetails, report.ErrorCategories, result.Err)
		} else {
			report.SuccessCount++
		}
		if onProgress != nil {
			update := ProgressUpdate{Completed: report.SuccessCount, Errors: report.ErrorCount}
			if result.Err != nil {
				update.ErrorDetail = result.Err.Error()
				update.ErrorCategory = ClassifyError(result.Err)
			}
			onProgress(update)
		}
	}

	report.WallTime = time.Since(start)
	return finalizeCacheHitReport(report)
}

func doCacheHitRequest(
	ctx context.Context,
	client *http.Client,
	provider ProviderConfig,
	cfg CacheHitConfig,
	userPrompt string,
	index int,
) CacheHitResult {
	result := CacheHitResult{Index: index}
	messages := []completionMessage{}
	if cfg.SystemPrompt != "" {
		messages = append(messages, completionMessage{Role: "system", Content: cfg.SystemPrompt})
	}
	messages = append(messages, completionMessage{Role: "user", Content: userPrompt})

	payload := completionRequest{
		Model:    provider.Model,
		Messages: messages,
		Stream:   false,
	}
	if cfg.MaxOutputTokens > 0 {
		payload.MaxTokens = cfg.MaxOutputTokens
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		result.Err = fmt.Errorf("failed to marshal request: %w", err)
		return result
	}
	reqBody = MergeCustomParams(reqBody, provider.CustomParams)

	req, err := http.NewRequestWithContext(ctx, "POST", provider.URL, bytes.NewBuffer(reqBody))
	if err != nil {
		result.Err = fmt.Errorf("failed to create request: %w", err)
		return result
	}
	req.Header.Set("Content-Type", "application/json")
	if provider.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKey)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.Err = err
		return result
	}
	defer resp.Body.Close()
	result.Latency = time.Since(start)

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Err = fmt.Errorf("failed to read response: %w", err)
		return result
	}
	if resp.StatusCode != http.StatusOK {
		snippet := strings.TrimSpace(string(rawBody))
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		if snippet == "" {
			result.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			result.Err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
		}
		return result
	}

	var parsed cacheHitResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		result.Err = fmt.Errorf("failed to parse response JSON: %w", err)
		return result
	}
	if parsed.Usage == nil {
		return result
	}

	result.PromptTokens = parsed.Usage.PromptTokens
	result.CompletionTokens = parsed.Usage.CompletionTokens
	result.TotalTokens = parsed.Usage.TotalTokens
	if parsed.Usage.PromptDetails != nil && parsed.Usage.PromptDetails.CachedTokens != nil {
		result.CachedTokens = *parsed.Usage.PromptDetails.CachedTokens
		result.HasCachedTokens = true
	}
	return result
}

func doAnthropicCacheHitRequest(
	ctx context.Context,
	client *http.Client,
	provider ProviderConfig,
	cfg CacheHitConfig,
	userPrompt string,
	index int,
) CacheHitResult {
	result := CacheHitResult{Index: index}

	maxTokens := cfg.MaxOutputTokens
	if maxTokens == 0 {
		maxTokens = anthropicDefaultMaxTokens
	}

	payload := map[string]any{
		"model":      provider.Model,
		"max_tokens": maxTokens,
		"stream":     false,
		"messages": []map[string]any{
			{"role": "user", "content": userPrompt},
		},
	}
	if cfg.SystemPrompt != "" {
		payload["system"] = cfg.SystemPrompt
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		result.Err = fmt.Errorf("failed to marshal request: %w", err)
		return result
	}
	reqBody = MergeCustomParams(reqBody, provider.CustomParams)
	reqBody = ensureAnthropicCacheControl(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", provider.URL, bytes.NewBuffer(reqBody))
	if err != nil {
		result.Err = fmt.Errorf("failed to create request: %w", err)
		return result
	}
	setAnthropicHeaders(req, provider.APIKey)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		result.Err = err
		return result
	}
	defer resp.Body.Close()
	result.Latency = time.Since(start)

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Err = fmt.Errorf("failed to read response: %w", err)
		return result
	}
	if resp.StatusCode != http.StatusOK {
		snippet := strings.TrimSpace(string(rawBody))
		if len(snippet) > 512 {
			snippet = snippet[:512]
		}
		if snippet == "" {
			result.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			result.Err = fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
		}
		return result
	}

	var parsed anthropicCacheHitResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		result.Err = fmt.Errorf("failed to parse response JSON: %w", err)
		return result
	}
	if parsed.Usage == nil {
		return result
	}

	if parsed.Usage.InputTokens != nil {
		result.PromptTokens = *parsed.Usage.InputTokens
	}
	if parsed.Usage.OutputTokens != nil {
		result.CompletionTokens = *parsed.Usage.OutputTokens
	}
	result.TotalTokens = result.PromptTokens + result.CompletionTokens
	if parsed.Usage.CacheReadInputTokens != nil {
		result.CachedTokens = *parsed.Usage.CacheReadInputTokens
		result.HasCachedTokens = true
	}
	if parsed.Usage.CacheCreationInputTokens != nil {
		result.CacheCreationTokens = *parsed.Usage.CacheCreationInputTokens
		result.HasCacheCreation = true
	}
	return result
}

// ensureAnthropicCacheControl injects top-level cache_control if Custom Params
// (or the base payload) did not already set it. Anthropic applies top-level
// cache_control to the last cacheable block automatically.
func ensureAnthropicCacheControl(body []byte) []byte {
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	if _, ok := obj["cache_control"]; ok {
		return body
	}
	obj["cache_control"] = map[string]any{"type": "ephemeral"}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

// cacheHitInputTotal returns the denominator for hit-rate calculation.
// Completions: prompt tokens. Anthropic: input + cache_creation + cache_read.
func cacheHitInputTotal(r CacheHitResult, apiMode string) int {
	if apiMode == ModeAnthropicMessages {
		return r.PromptTokens + r.CacheCreationTokens + r.CachedTokens
	}
	return r.PromptTokens
}

func finalizeCacheHitReport(report CacheHitReport) CacheHitReport {
	var (
		latencyTotal time.Duration
		rateTotal    float64
		rateCount    int
		totalInput   int
	)
	for _, result := range report.Results {
		if result.Err != nil {
			continue
		}
		latencyTotal += result.Latency
		report.TotalPromptTokens += result.PromptTokens
		report.TotalCacheCreationTokens += result.CacheCreationTokens
		inputTotal := cacheHitInputTotal(result, report.APIMode)
		totalInput += inputTotal
		// Completions: missing usage = all OpenAI usage totals are zero.
		// Anthropic: also treat presence of cache_read/creation as having usage.
		if report.APIMode == ModeAnthropicMessages {
			if result.PromptTokens == 0 && result.CompletionTokens == 0 && result.TotalTokens == 0 &&
				!result.HasCachedTokens && !result.HasCacheCreation {
				report.MissingUsageCount++
			}
		} else if result.PromptTokens == 0 && result.CompletionTokens == 0 && result.TotalTokens == 0 {
			report.MissingUsageCount++
		}
		if result.HasCachedTokens {
			report.TotalCachedTokens += result.CachedTokens
			if inputTotal > 0 {
				rateTotal += float64(result.CachedTokens) / float64(inputTotal) * 100
				rateCount++
			}
		} else {
			report.MissingCachedCount++
		}
	}
	if report.SuccessCount > 0 {
		report.Valid = true
		report.AvgLatencyMs = float64(latencyTotal.Milliseconds()) / float64(report.SuccessCount)
	}
	if totalInput > 0 {
		report.CacheHitRate = float64(report.TotalCachedTokens) / float64(totalInput) * 100
	}
	if rateCount > 0 {
		report.AvgRequestHitRate = rateTotal / float64(rateCount)
	}
	return report
}
