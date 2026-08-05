package bench

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestBenchInput(t *testing.T) {
	if got := benchInput([]string{"only"}, 5); got != "only" {
		t.Fatalf("same-mode pick = %q, want only", got)
	}
	texts := []string{"a", "b", "c"}
	if got := benchInput(texts, 1); got != "b" {
		t.Fatalf("unique-mode pick = %q, want b", got)
	}
}

func TestRunEmbeddingBench_UniqueInputsSendDistinctBodies(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1]}]}`))
	}))
	defer server.Close()

	texts := []string{"prompt-alpha", "prompt-beta", "prompt-gamma"}
	report := RunEmbeddingBench(
		context.Background(),
		ProviderConfig{URL: server.URL, Model: "test-model"},
		BenchConfig{Concurrency: 2, TotalRequests: 3, UniqueInputs: true},
		texts,
		1,
		nil,
	)
	if report.SuccessCount != 3 {
		t.Fatalf("SuccessCount = %d, want 3; errors=%v", report.SuccessCount, report.ErrorDetails)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 3 {
		t.Fatalf("captured bodies = %d, want 3", len(bodies))
	}
	seen := map[string]bool{}
	for _, body := range bodies {
		var req embeddingRequest
		if err := json.Unmarshal([]byte(body), &req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(req.Input) != 1 {
			t.Fatalf("input len = %d, want 1", len(req.Input))
		}
		seen[req.Input[0]] = true
	}
	for _, want := range texts {
		if !seen[want] {
			t.Errorf("missing unique input %q in requests; seen=%v", want, seen)
		}
	}
}
