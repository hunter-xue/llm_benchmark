package bench

import (
	"math"
	"time"
)

// EmbeddingReport holds aggregated results from an embedding benchmark run.
type EmbeddingReport struct {
	TotalRequests int
	SuccessCount  int
	ErrorCount    int
	WallTime      time.Duration
	RPS           float64
	InputTPS      float64
	InputTPM      float64
	LatencyAvg    float64
	LatencyP50    float64
	LatencyP90    float64
	LatencyP99    float64
	ErrorDetails  map[string]int
	Valid         bool // false if all requests failed
}

// CompletionReport holds aggregated results from a completion benchmark run.
type CompletionReport struct {
	TotalRequests   int
	SuccessCount    int
	ErrorCount      int
	WallTime        time.Duration
	RPS             float64
	InputTPS        float64
	OutputTPS       float64
	InputTPM        float64
	OutputTPM       float64
	AvgOutputTokens float64
	TTFTAvg         float64
	TTFTp50         float64
	TTFTp90         float64
	TTFTp99         float64
	TPOTAvg         float64
	TPOTp50         float64
	TPOTp90         float64
	TPOTp99         float64
	E2EAvg          float64
	E2Ep50          float64
	E2Ep90          float64
	E2Ep99          float64
	SkippedChunks   int
	ErrorDetails    map[string]int
	Valid           bool
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
