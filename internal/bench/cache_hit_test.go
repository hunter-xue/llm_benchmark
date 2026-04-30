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
}

func jsonInt(v int) string {
	b, _ := json.Marshal(v)
	return string(b)
}
