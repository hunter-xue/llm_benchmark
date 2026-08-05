package bench

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunEmbeddingBenchClassifiesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	report := RunEmbeddingBench(
		context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		BenchConfig{Concurrency: 1, TotalRequests: 2},
		[]string{"hello"},
		1,
		nil,
	)

	if got := report.ErrorCategories[ErrorCategoryRateLimit]; got != 2 {
		t.Fatalf("rate limit category count = %d, want 2", got)
	}
}

func TestRunCompletionBenchClassifiesEmptyOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	report := RunCompletionBench(
		context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		BenchConfig{Concurrency: 1, TotalRequests: 1},
		[]string{"hello"},
		1,
		nil,
		nil,
	)

	if got := report.ErrorCategories[ErrorCategoryEmptyOutput]; got != 1 {
		t.Fatalf("empty output category count = %d, want 1", got)
	}
}

func TestRunCompletionBenchExtractsStreamingAPIUsage(t *testing.T) {
	tkm := testTokenizer(t)
	var checkedRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		streamOptions, ok := body["stream_options"].(map[string]any)
		if !ok {
			t.Fatalf("stream_options missing or invalid: %#v", body["stream_options"])
		}
		if streamOptions["include_usage"] != true {
			t.Fatalf("include_usage = %#v, want true", streamOptions["include_usage"])
		}
		checkedRequest = true

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":14,\"completion_tokens\":7,\"total_tokens\":21}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	report := RunCompletionBench(
		context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		BenchConfig{Concurrency: 1, TotalRequests: 1},
		[]string{"hello"},
		1,
		tkm,
		nil,
	)

	if !checkedRequest {
		t.Fatalf("server did not check request body")
	}
	if report.SuccessCount != 1 || !report.Valid {
		t.Fatalf("success/valid = %d/%v, want 1/true", report.SuccessCount, report.Valid)
	}
	if report.APIPromptTokens != 14 || report.APICompletionTokens != 7 || report.APITotalTokens != 21 {
		t.Fatalf("api tokens = %d/%d/%d, want 14/7/21", report.APIPromptTokens, report.APICompletionTokens, report.APITotalTokens)
	}
	if report.APIUsageCount != 1 || report.MissingAPIUsageCount != 0 {
		t.Fatalf("usage/missing = %d/%d, want 1/0", report.APIUsageCount, report.MissingAPIUsageCount)
	}
}

func TestRunCompletionBenchCountsMissingAPIUsage(t *testing.T) {
	tkm := testTokenizer(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	report := RunCompletionBench(
		context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		BenchConfig{Concurrency: 1, TotalRequests: 1},
		[]string{"hello"},
		1,
		tkm,
		nil,
	)

	if report.SuccessCount != 1 || !report.Valid {
		t.Fatalf("success/valid = %d/%v, want 1/true", report.SuccessCount, report.Valid)
	}
	if report.APIUsageCount != 0 || report.MissingAPIUsageCount != 1 {
		t.Fatalf("usage/missing = %d/%d, want 0/1", report.APIUsageCount, report.MissingAPIUsageCount)
	}
}

func TestRunAnthropicMessagesBenchClassifiesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	report := RunAnthropicMessagesBench(
		context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		BenchConfig{Concurrency: 1, TotalRequests: 1},
		[]string{"hello"},
		1,
		nil,
		nil,
	)

	if got := report.ErrorCategories[ErrorCategoryRateLimit]; got != 1 {
		t.Fatalf("rate limit category count = %d, want 1", got)
	}
}

func TestRunAnthropicMessagesBenchExtractsStreamingAPIUsage(t *testing.T) {
	tkm := testTokenizer(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":14,\"output_tokens\":1}}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	report := RunAnthropicMessagesBench(
		context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		BenchConfig{Concurrency: 1, TotalRequests: 1},
		[]string{"hello"},
		1,
		tkm,
		nil,
	)

	if report.SuccessCount != 1 || !report.Valid {
		t.Fatalf("success/valid = %d/%v, want 1/true", report.SuccessCount, report.Valid)
	}
	if report.APIPromptTokens != 14 || report.APICompletionTokens != 7 || report.APITotalTokens != 21 {
		t.Fatalf("api tokens = %d/%d/%d, want 14/7/21", report.APIPromptTokens, report.APICompletionTokens, report.APITotalTokens)
	}
	if report.APIUsageCount != 1 || report.MissingAPIUsageCount != 0 {
		t.Fatalf("usage/missing = %d/%d, want 1/0", report.APIUsageCount, report.MissingAPIUsageCount)
	}
}
