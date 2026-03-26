package bench

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

const (
	ModeEmbedding  = "embedding"
	ModeCompletion = "completion"
)

// ProviderConfig holds the per-provider settings used in benchmarks.
type ProviderConfig struct {
	Name         string // display name, used in PK mode
	URL          string // full API endpoint URL
	APIKey       string
	Model        string
	CustomParams string // optional JSON object merged into every request body
}

// BenchConfig holds shared benchmark parameters.
type BenchConfig struct {
	Mode            string // "embedding" or "completion"
	Concurrency     int
	TotalRequests   int
	TargetTokens    int
	MaxOutputTokens int    // completion only, 0 = unlimited
	SystemPrompt    string // completion only
}

// MergeCustomParams merges a JSON object string into an already-marshaled request
// body. Keys in customParams override existing keys. Returns body unchanged on
// any error so the request still proceeds with the standard payload.
func MergeCustomParams(body []byte, customParams string) []byte {
	if customParams == "" {
		return body
	}
	var base map[string]any
	if err := json.Unmarshal(body, &base); err != nil {
		return body
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(customParams), &extra); err != nil {
		return body
	}
	for k, v := range extra {
		base[k] = v
	}
	merged, err := json.Marshal(base)
	if err != nil {
		return body
	}
	return merged
}

// DetectMode inspects the URL path to determine embedding vs completion mode.
func DetectMode(apiURL string) string {
	parsed, err := url.Parse(apiURL)
	if err != nil {
		return ModeEmbedding
	}
	p := strings.ToLower(parsed.Path)
	if strings.Contains(p, "chat/completions") || strings.HasSuffix(p, "/completions") {
		return ModeCompletion
	}
	return ModeEmbedding
}

// NormalizeURL validates and normalizes a raw API URL string.
func NormalizeURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("URL must not be empty")
	}
	if !(strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://")) {
		return "", fmt.Errorf("URL must start with http:// or https://")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("URL must include a scheme and host, e.g. http://127.0.0.1:3000/v1/embeddings")
	}
	return parsed.String(), nil
}
