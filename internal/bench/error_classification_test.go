package bench

import (
	"context"
	"errors"
	"net/url"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"rate limit", errors.New("HTTP 429: rate limited"), ErrorCategoryRateLimit},
		{"auth unauthorized", errors.New("HTTP 401: unauthorized"), ErrorCategoryAuth},
		{"auth forbidden", errors.New("HTTP 403: forbidden"), ErrorCategoryAuth},
		{"bad request", errors.New("HTTP 400: bad request"), ErrorCategoryBadRequest},
		{"server error", errors.New("HTTP 502: bad gateway"), ErrorCategoryServer},
		{"timeout", context.DeadlineExceeded, ErrorCategoryTimeout},
		{"timeout text", errors.New("Post \"http://example.test\": i/o timeout"), ErrorCategoryTimeout},
		{"connection", &url.Error{Op: "Post", URL: "http://127.0.0.1", Err: errors.New("connect: connection refused")}, ErrorCategoryConnection},
		{"empty output", errors.New("no output tokens received"), ErrorCategoryEmptyOutput},
		{"reasoning only output", errors.New("only reasoning tokens received, no answer content (thinking exhausted max_tokens?)"), ErrorCategoryEmptyOutput},
		{"stream error", errors.New("failed to read stream: unexpected EOF"), ErrorCategoryStream},
		{"parse error", errors.New("failed to parse response JSON: invalid character"), ErrorCategoryParse},
		{"client error", errors.New("failed to create request: missing protocol scheme"), ErrorCategoryClient},
		{"other", errors.New("something else"), ErrorCategoryOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.err); got != tt.want {
				t.Fatalf("ClassifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
