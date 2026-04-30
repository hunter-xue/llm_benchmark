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
	TestCount       int
	MaxOutputTokens int
	SystemPrompt    string
	Interval        time.Duration
}

type CacheHitResult struct {
	Index            int
	Latency          time.Duration
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int
	HasCachedTokens  bool
	Err              error
}

type CacheHitReport struct {
	TotalRequests      int
	SuccessCount       int
	ErrorCount         int
	WallTime           time.Duration
	Results            []CacheHitResult
	TotalPromptTokens  int
	TotalCachedTokens  int
	CacheHitRate       float64
	AvgRequestHitRate  float64
	AvgLatencyMs       float64
	MissingUsageCount  int
	MissingCachedCount int
	ErrorDetails       map[string]int
	Valid              bool
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

// RunCacheHitTest sends repeated non-streaming Chat Completions requests and
// extracts prompt cache hits from usage.prompt_tokens_details.cached_tokens.
func RunCacheHitTest(
	ctx context.Context,
	provider ProviderConfig,
	cfg CacheHitConfig,
	userPrompt string,
	onProgress func(completed, errors int),
) CacheHitReport {
	if cfg.Interval <= 0 {
		cfg.Interval = 3 * time.Second
	}

	report := CacheHitReport{
		TotalRequests: cfg.TestCount,
		Results:       make([]CacheHitResult, 0, cfg.TestCount),
		ErrorDetails:  make(map[string]int),
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
				report.ErrorDetails[ctx.Err().Error()]++
				if onProgress != nil {
					onProgress(report.SuccessCount, report.ErrorCount)
				}
				report.WallTime = time.Since(start)
				return finalizeCacheHitReport(report)
			case <-timer.C:
			}
		}

		result := doCacheHitRequest(ctx, client, provider, cfg, userPrompt, i+1)
		report.Results = append(report.Results, result)
		if result.Err != nil {
			report.ErrorCount++
			report.ErrorDetails[result.Err.Error()]++
		} else {
			report.SuccessCount++
		}
		if onProgress != nil {
			onProgress(report.SuccessCount, report.ErrorCount)
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

func finalizeCacheHitReport(report CacheHitReport) CacheHitReport {
	var (
		latencyTotal time.Duration
		rateTotal    float64
		rateCount    int
	)
	for _, result := range report.Results {
		if result.Err != nil {
			continue
		}
		latencyTotal += result.Latency
		report.TotalPromptTokens += result.PromptTokens
		if result.PromptTokens == 0 && result.CompletionTokens == 0 && result.TotalTokens == 0 {
			report.MissingUsageCount++
		}
		if result.HasCachedTokens {
			report.TotalCachedTokens += result.CachedTokens
			if result.PromptTokens > 0 {
				rateTotal += float64(result.CachedTokens) / float64(result.PromptTokens) * 100
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
	if report.TotalPromptTokens > 0 {
		report.CacheHitRate = float64(report.TotalCachedTokens) / float64(report.TotalPromptTokens) * 100
	}
	if rateCount > 0 {
		report.AvgRequestHitRate = rateTotal / float64(rateCount)
	}
	return report
}
