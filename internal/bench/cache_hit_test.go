package bench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunCacheHitTestExtractsCachedTokens(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["stream"] != false {
			t.Fatalf("stream = %v, want false", body["stream"])
		}
		w.Header().Set("Content-Type", "application/json")
		cached := 0
		if requests > 1 {
			cached = 1024
		}
		_, _ = w.Write([]byte(`{
			"usage": {
				"prompt_tokens": 1280,
				"completion_tokens": 1,
				"total_tokens": 1281,
				"prompt_tokens_details": {"cached_tokens": ` + jsonInt(cached) + `}
			}
		}`))
	}))
	defer server.Close()

	report := RunCacheHitTest(context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		CacheHitConfig{TestCount: 3, MaxOutputTokens: 1, Interval: time.Nanosecond},
		"large prompt",
		nil,
	)

	if report.SuccessCount != 3 || report.ErrorCount != 0 {
		t.Fatalf("success/errors = %d/%d, want 3/0", report.SuccessCount, report.ErrorCount)
	}
	if len(report.Results) != 3 {
		t.Fatalf("results len = %d, want 3", len(report.Results))
	}
	if report.TotalPromptTokens != 3840 {
		t.Fatalf("total prompt tokens = %d, want 3840", report.TotalPromptTokens)
	}
	if report.TotalCachedTokens != 2048 {
		t.Fatalf("total cached tokens = %d, want 2048", report.TotalCachedTokens)
	}
	if report.MissingCachedCount != 0 {
		t.Fatalf("missing cached count = %d, want 0", report.MissingCachedCount)
	}
	if !report.Valid {
		t.Fatalf("report should be valid")
	}
}

func TestRunCacheHitTestMissingUsageAndCachedTokens(t *testing.T) {
	responses := []string{
		`{"choices":[]}`,
		`{"usage":{"prompt_tokens":100,"completion_tokens":1,"total_tokens":101}}`,
	}
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[requests]))
		requests++
	}))
	defer server.Close()

	report := RunCacheHitTest(context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		CacheHitConfig{TestCount: 2, Interval: time.Nanosecond},
		"prompt",
		nil,
	)

	if report.SuccessCount != 2 || report.ErrorCount != 0 {
		t.Fatalf("success/errors = %d/%d, want 2/0", report.SuccessCount, report.ErrorCount)
	}
	if report.MissingUsageCount != 1 {
		t.Fatalf("missing usage count = %d, want 1", report.MissingUsageCount)
	}
	if report.MissingCachedCount != 2 {
		t.Fatalf("missing cached count = %d, want 2", report.MissingCachedCount)
	}
}

func TestRunCacheHitTestRecordsErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	report := RunCacheHitTest(context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		CacheHitConfig{TestCount: 1, Interval: time.Nanosecond},
		"prompt",
		nil,
	)

	if report.SuccessCount != 0 || report.ErrorCount != 1 {
		t.Fatalf("success/errors = %d/%d, want 0/1", report.SuccessCount, report.ErrorCount)
	}
	if len(report.ErrorDetails) != 1 {
		t.Fatalf("error details len = %d, want 1", len(report.ErrorDetails))
	}
	if got := report.ErrorCategories[ErrorCategoryRateLimit]; got != 1 {
		t.Fatalf("rate limit category count = %d, want 1", got)
	}
}

func TestRunCacheHitTestAnthropicCacheTokens(t *testing.T) {
	var requests int
	var sawCacheControl bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("x-api-key") == "" {
			t.Error("expected x-api-key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("expected anthropic-version header")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["stream"] != false {
			t.Fatalf("stream = %v, want false", body["stream"])
		}
		if _, ok := body["cache_control"]; ok {
			sawCacheControl = true
		}
		w.Header().Set("Content-Type", "application/json")
		cacheRead := 0
		cacheCreate := 1024
		if requests > 1 {
			cacheRead = 1024
			cacheCreate = 0
		}
		_, _ = w.Write([]byte(`{
			"usage": {
				"input_tokens": 200,
				"output_tokens": 1,
				"cache_creation_input_tokens": ` + jsonInt(cacheCreate) + `,
				"cache_read_input_tokens": ` + jsonInt(cacheRead) + `
			}
		}`))
	}))
	defer server.Close()

	report := RunCacheHitTest(context.Background(),
		ProviderConfig{URL: server.URL, APIKey: "sk-ant-test", Model: "claude-test"},
		CacheHitConfig{
			APIMode:         ModeAnthropicMessages,
			TestCount:       2,
			MaxOutputTokens: 1,
			Interval:        time.Nanosecond,
		},
		"large prompt",
		nil,
	)

	if !sawCacheControl {
		t.Fatal("expected cache_control to be injected into Anthropic requests")
	}
	if report.APIMode != ModeAnthropicMessages {
		t.Fatalf("APIMode = %q, want %q", report.APIMode, ModeAnthropicMessages)
	}
	if report.SuccessCount != 2 {
		t.Fatalf("success = %d, want 2", report.SuccessCount)
	}
	if report.TotalPromptTokens != 400 {
		t.Fatalf("total input tokens = %d, want 400", report.TotalPromptTokens)
	}
	if report.TotalCachedTokens != 1024 {
		t.Fatalf("total cache read = %d, want 1024", report.TotalCachedTokens)
	}
	if report.TotalCacheCreationTokens != 1024 {
		t.Fatalf("total cache creation = %d, want 1024", report.TotalCacheCreationTokens)
	}
	// Hit rate denom = (200+1024+0) + (200+0+1024) = 2448; cached = 1024
	wantRate := float64(1024) / float64(2448) * 100
	if report.CacheHitRate < wantRate-0.01 || report.CacheHitRate > wantRate+0.01 {
		t.Fatalf("CacheHitRate = %.4f, want ~%.4f", report.CacheHitRate, wantRate)
	}
	if !report.Results[0].HasCacheCreation || report.Results[0].CacheCreationTokens != 1024 {
		t.Fatalf("request 1 creation = %+v", report.Results[0])
	}
	if !report.Results[1].HasCachedTokens || report.Results[1].CachedTokens != 1024 {
		t.Fatalf("request 2 cached = %+v", report.Results[1])
	}
}

func TestEnsureAnthropicCacheControl_SkipsWhenPresent(t *testing.T) {
	in := []byte(`{"model":"m","cache_control":{"type":"ephemeral","ttl":"1h"}}`)
	out := ensureAnthropicCacheControl(in)
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatal(err)
	}
	cc := obj["cache_control"].(map[string]any)
	if cc["ttl"] != "1h" {
		t.Fatalf("cache_control overwritten: %+v", cc)
	}
}

func jsonInt(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
