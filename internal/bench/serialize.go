package bench

import (
	"encoding/json"
	"encoding/xml"
	"time"
)

// --- Embedding output types ---

type EmbeddingOutput struct {
	XMLName       xml.Name          `json:"-" xml:"embedding_report"`
	Name          string            `json:"name,omitempty" xml:"name,omitempty"`
	TotalRequests int               `json:"total_requests" xml:"total_requests"`
	SuccessCount  int               `json:"success_count" xml:"success_count"`
	ErrorCount    int               `json:"error_count" xml:"error_count"`
	WallTimeSec   float64           `json:"wall_time_seconds" xml:"wall_time_seconds"`
	RPS           float64           `json:"rps" xml:"rps"`
	InputTPS      float64           `json:"input_tps" xml:"input_tps"`
	InputTPM      float64           `json:"input_tpm" xml:"input_tpm"`
	LatencyAvgMs  float64           `json:"latency_avg_ms" xml:"latency_avg_ms"`
	LatencyP50Ms  float64           `json:"latency_p50_ms" xml:"latency_p50_ms"`
	LatencyP90Ms  float64           `json:"latency_p90_ms" xml:"latency_p90_ms"`
	LatencyP99Ms  float64           `json:"latency_p99_ms" xml:"latency_p99_ms"`
	ErrorDetails  []ErrorDetailItem `json:"error_details" xml:"errors>error"`
	Valid         bool              `json:"valid" xml:"valid"`
}

// --- Completion output types ---

type CompletionOutput struct {
	XMLName        xml.Name          `json:"-" xml:"completion_report"`
	Name           string            `json:"name,omitempty" xml:"name,omitempty"`
	TotalRequests  int               `json:"total_requests" xml:"total_requests"`
	SuccessCount   int               `json:"success_count" xml:"success_count"`
	ErrorCount     int               `json:"error_count" xml:"error_count"`
	WallTimeSec    float64           `json:"wall_time_seconds" xml:"wall_time_seconds"`
	RPS            float64           `json:"rps" xml:"rps"`
	InputTPS       float64           `json:"input_tps" xml:"input_tps"`
	OutputTPS      float64           `json:"output_tps" xml:"output_tps"`
	InputTPM       float64           `json:"input_tpm" xml:"input_tpm"`
	OutputTPM      float64           `json:"output_tpm" xml:"output_tpm"`
	AvgOutputTok   float64           `json:"avg_output_tokens" xml:"avg_output_tokens"`
	TTFTAvgMs      float64           `json:"ttft_avg_ms" xml:"ttft_avg_ms"`
	TTFTP50Ms      float64           `json:"ttft_p50_ms" xml:"ttft_p50_ms"`
	TTFTP90Ms      float64           `json:"ttft_p90_ms" xml:"ttft_p90_ms"`
	TTFTP99Ms      float64           `json:"ttft_p99_ms" xml:"ttft_p99_ms"`
	TPOTAvgMs      float64           `json:"tpot_avg_ms" xml:"tpot_avg_ms"`
	TPOTP50Ms      float64           `json:"tpot_p50_ms" xml:"tpot_p50_ms"`
	TPOTP90Ms      float64           `json:"tpot_p90_ms" xml:"tpot_p90_ms"`
	TPOTP99Ms      float64           `json:"tpot_p99_ms" xml:"tpot_p99_ms"`
	E2EAvgMs       float64           `json:"e2e_avg_ms" xml:"e2e_avg_ms"`
	E2EP50Ms       float64           `json:"e2e_p50_ms" xml:"e2e_p50_ms"`
	E2EP90Ms       float64           `json:"e2e_p90_ms" xml:"e2e_p90_ms"`
	E2EP99Ms       float64           `json:"e2e_p99_ms" xml:"e2e_p99_ms"`
	SkippedChunks  int               `json:"skipped_chunks" xml:"skipped_chunks"`
	ErrorDetails   []ErrorDetailItem `json:"error_details" xml:"errors>error"`
	Valid          bool              `json:"valid" xml:"valid"`
}

// --- Cache hit output types ---

type CacheHitResultOutput struct {
	XMLName          xml.Name `json:"-" xml:"result"`
	Index            int      `json:"index" xml:"index,attr"`
	LatencyMs        float64  `json:"latency_ms" xml:"latency_ms"`
	PromptTokens     int      `json:"prompt_tokens" xml:"prompt_tokens"`
	CompletionTokens int      `json:"completion_tokens" xml:"completion_tokens"`
	TotalTokens      int      `json:"total_tokens" xml:"total_tokens"`
	CachedTokens     int      `json:"cached_tokens" xml:"cached_tokens"`
	HasCachedTokens  bool     `json:"has_cached_tokens" xml:"has_cached_tokens"`
	Error            string   `json:"error,omitempty" xml:"error,omitempty"`
}

type CacheHitOutput struct {
	XMLName         xml.Name              `json:"-" xml:"cache_hit_report"`
	TotalRequests   int                   `json:"total_requests" xml:"total_requests"`
	SuccessCount    int                   `json:"success_count" xml:"success_count"`
	ErrorCount      int                   `json:"error_count" xml:"error_count"`
	WallTimeSec     float64              `json:"wall_time_seconds" xml:"wall_time_seconds"`
	Results         []CacheHitResultOutput `json:"results" xml:"results>result"`
	TotalPromptTok  int                  `json:"total_prompt_tokens" xml:"total_prompt_tokens"`
	TotalCachedTok  int                  `json:"total_cached_tokens" xml:"total_cached_tokens"`
	CacheHitRate    float64              `json:"cache_hit_rate" xml:"cache_hit_rate"`
	AvgReqHitRate   float64              `json:"avg_request_hit_rate" xml:"avg_request_hit_rate"`
	AvgLatencyMs    float64              `json:"avg_latency_ms" xml:"avg_latency_ms"`
	MissingUsageCnt int                  `json:"missing_usage_count" xml:"missing_usage_count"`
	MissingCachedCnt int                 `json:"missing_cached_count" xml:"missing_cached_count"`
	ErrorDetails    []ErrorDetailItem    `json:"error_details" xml:"errors>error"`
	Valid           bool                 `json:"valid" xml:"valid"`
}

// --- PK mode wrapper types ---

type EmbeddingPKOutput struct {
	XMLName   xml.Name        `json:"-" xml:"pk_report"`
	ProviderA EmbeddingOutput `json:"provider_a" xml:"provider_a"`
	ProviderB EmbeddingOutput `json:"provider_b" xml:"provider_b"`
}

type CompletionPKOutput struct {
	XMLName   xml.Name         `json:"-" xml:"pk_report"`
	ProviderA CompletionOutput `json:"provider_a" xml:"provider_a"`
	ProviderB CompletionOutput `json:"provider_b" xml:"provider_b"`
}

// --- Compare output types ---

type CompareProviderResult struct {
	XMLName xml.Name `json:"-" xml:"provider_result"`
	Name    string   `json:"name" xml:"name"`
	Headers string   `json:"headers" xml:"headers"`
	Body    string   `json:"-" xml:"body"` // internal: stored as string; MarshalJSON inlines as JSON object
	Error   string   `json:"error,omitempty" xml:"error,omitempty"`
}

func (r CompareProviderResult) MarshalJSON() ([]byte, error) {
	var body any
	if json.Valid([]byte(r.Body)) {
		body = json.RawMessage(r.Body)
	} else {
		body = r.Body
	}
	return json.Marshal(struct {
		Name    string `json:"name"`
		Headers string `json:"headers"`
		Body    any    `json:"body"`
		Error   string `json:"error,omitempty"`
	}{
		Name:    r.Name,
		Headers: r.Headers,
		Body:    body,
		Error:   r.Error,
	})
}

type CompareOutput struct {
	XMLName     xml.Name              `json:"-" xml:"compare_report"`
	UserMessage string                `json:"user_message" xml:"user_message"`
	ProviderA   CompareProviderResult `json:"provider_a" xml:"provider_a"`
	ProviderB   CompareProviderResult `json:"provider_b" xml:"provider_b"`
	Valid       bool                  `json:"valid" xml:"valid"`
}

// --- Top-level benchmark result wrapper ---

type BenchmarkResult struct {
	Mode     string
	TestMode string
	// Exactly one of these should be populated
	singleEmbedding  *EmbeddingOutput
	singleCompletion *CompletionOutput
	pkEmbedding      *EmbeddingPKOutput
	pkCompletion      *CompletionPKOutput
	compare          *CompareOutput
	cacheHit         *CacheHitOutput
}

func (r BenchmarkResult) MarshalJSON() ([]byte, error) {
	m := map[string]any{
		"mode":      r.Mode,
		"test_mode": r.TestMode,
	}
	switch r.TestMode {
	case "single":
		if r.singleEmbedding != nil {
			m["single"] = r.singleEmbedding
		} else if r.singleCompletion != nil {
			m["single"] = r.singleCompletion
		}
	case "pk":
		if r.pkEmbedding != nil {
			m["pk"] = r.pkEmbedding
		} else if r.pkCompletion != nil {
			m["pk"] = r.pkCompletion
		}
	case "compare":
		if r.compare != nil {
			m["compare"] = r.compare
		}
	case "cache_hit":
		if r.cacheHit != nil {
			m["cache_hit"] = r.cacheHit
		}
	}
	return json.Marshal(m)
}

func (r BenchmarkResult) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "benchmark_result"}
	if err := e.EncodeToken(start); err != nil {
		return err
	}

	e.EncodeElement(r.Mode, xml.StartElement{Name: xml.Name{Local: "mode"}})
	e.EncodeElement(r.TestMode, xml.StartElement{Name: xml.Name{Local: "test_mode"}})

	var content any
	var contentKey string
	switch r.TestMode {
	case "single":
		contentKey = "single"
		if r.singleEmbedding != nil {
			content = r.singleEmbedding
		} else if r.singleCompletion != nil {
			content = r.singleCompletion
		}
	case "pk":
		contentKey = "pk"
		if r.pkEmbedding != nil {
			content = r.pkEmbedding
		} else if r.pkCompletion != nil {
			content = r.pkCompletion
		}
	case "compare":
		contentKey = "compare"
		content = r.compare
	case "cache_hit":
		contentKey = "cache_hit"
		content = r.cacheHit
	}

	if content != nil {
		e.EncodeElement(content, xml.StartElement{Name: xml.Name{Local: contentKey}})
	}

	return e.EncodeToken(start.End())
}

// --- Error detail item (map→slice adapter for XML) ---

type ErrorDetailItem struct {
	Message string `json:"message" xml:"message"`
	Count   int    `json:"count" xml:"count"`
}

func errorDetailsToItems(m map[string]int) []ErrorDetailItem {
	if len(m) == 0 {
		return nil
	}
	items := make([]ErrorDetailItem, 0, len(m))
	for msg, cnt := range m {
		items = append(items, ErrorDetailItem{Message: msg, Count: cnt})
	}
	return items
}

// --- Conversion functions ---

func NewEmbeddingOutput(r EmbeddingReport, name string) EmbeddingOutput {
	return EmbeddingOutput{
		Name:          name,
		TotalRequests: r.TotalRequests,
		SuccessCount:  r.SuccessCount,
		ErrorCount:    r.ErrorCount,
		WallTimeSec:   r.WallTime.Seconds(),
		RPS:           r.RPS,
		InputTPS:      r.InputTPS,
		InputTPM:      r.InputTPM,
		LatencyAvgMs:  r.LatencyAvg,
		LatencyP50Ms:  r.LatencyP50,
		LatencyP90Ms:  r.LatencyP90,
		LatencyP99Ms:  r.LatencyP99,
		ErrorDetails:  errorDetailsToItems(r.ErrorDetails),
		Valid:         r.Valid,
	}
}

func NewCompletionOutput(r CompletionReport, name string) CompletionOutput {
	return CompletionOutput{
		Name:           name,
		TotalRequests:  r.TotalRequests,
		SuccessCount:   r.SuccessCount,
		ErrorCount:     r.ErrorCount,
		WallTimeSec:    r.WallTime.Seconds(),
		RPS:            r.RPS,
		InputTPS:       r.InputTPS,
		OutputTPS:      r.OutputTPS,
		InputTPM:       r.InputTPM,
		OutputTPM:      r.OutputTPM,
		AvgOutputTok:   r.AvgOutputTokens,
		TTFTAvgMs:      r.TTFTAvg,
		TTFTP50Ms:      r.TTFTp50,
		TTFTP90Ms:      r.TTFTp90,
		TTFTP99Ms:      r.TTFTp99,
		TPOTAvgMs:      r.TPOTAvg,
		TPOTP50Ms:      r.TPOTp50,
		TPOTP90Ms:      r.TPOTp90,
		TPOTP99Ms:      r.TPOTp99,
		E2EAvgMs:       r.E2EAvg,
		E2EP50Ms:       r.E2Ep50,
		E2EP90Ms:       r.E2Ep90,
		E2EP99Ms:       r.E2Ep99,
		SkippedChunks:  r.SkippedChunks,
		ErrorDetails:   errorDetailsToItems(r.ErrorDetails),
		Valid:          r.Valid,
	}
}

func NewCacheHitOutput(r CacheHitReport) CacheHitOutput {
	out := CacheHitOutput{
		TotalRequests:   r.TotalRequests,
		SuccessCount:    r.SuccessCount,
		ErrorCount:      r.ErrorCount,
		WallTimeSec:     r.WallTime.Seconds(),
		TotalPromptTok:  r.TotalPromptTokens,
		TotalCachedTok:  r.TotalCachedTokens,
		CacheHitRate:    r.CacheHitRate,
		AvgReqHitRate:   r.AvgRequestHitRate,
		AvgLatencyMs:    r.AvgLatencyMs,
		MissingUsageCnt: r.MissingUsageCount,
		MissingCachedCnt: r.MissingCachedCount,
		ErrorDetails:    errorDetailsToItems(r.ErrorDetails),
		Valid:           r.Valid,
	}
	for _, res := range r.Results {
		item := CacheHitResultOutput{
			Index:            res.Index,
			PromptTokens:     res.PromptTokens,
			CompletionTokens: res.CompletionTokens,
			TotalTokens:      res.TotalTokens,
			CachedTokens:     res.CachedTokens,
			HasCachedTokens:  res.HasCachedTokens,
		}
		if res.Latency > 0 {
			item.LatencyMs = float64(res.Latency.Milliseconds())
		}
		if res.Err != nil {
			item.Error = res.Err.Error()
		}
		out.Results = append(out.Results, item)
	}
	return out
}

// MarshalJSON marshals EmbeddingReport with Duration converted to seconds and
// ErrorDetails as a slice.
func (r EmbeddingReport) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewEmbeddingOutput(r, ""))
}

// MarshalXML marshals EmbeddingReport as XML.
func (r EmbeddingReport) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(NewEmbeddingOutput(r, ""), start)
}

// MarshalJSON marshals CompletionReport with Duration converted to seconds and
// ErrorDetails as a slice.
func (r CompletionReport) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewCompletionOutput(r, ""))
}

// MarshalXML marshals CompletionReport as XML.
func (r CompletionReport) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(NewCompletionOutput(r, ""), start)
}

// MarshalJSON marshals CacheHitReport with proper field conversions.
func (r CacheHitReport) MarshalJSON() ([]byte, error) {
	return json.Marshal(NewCacheHitOutput(r))
}

// MarshalXML marshals CacheHitReport as XML.
func (r CacheHitReport) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	return e.EncodeElement(NewCacheHitOutput(r), start)
}

// MarshalBenchmarkResult serializes a BenchmarkResult to JSON or XML.
func MarshalBenchmarkResult(result BenchmarkResult, format string) ([]byte, error) {
	switch format {
	case "json":
		return json.MarshalIndent(result, "", "  ")
	case "xml":
		output, err := xml.Marshal(result)
		if err != nil {
			return nil, err
		}
		return append([]byte(xml.Header), output...), nil
	default:
		return nil, nil
	}
}

// Helper: build BenchmarkResult from single embedding report.
func SingleEmbeddingResult(mode, testMode string, report EmbeddingReport) BenchmarkResult {
	out := NewEmbeddingOutput(report, "")
	return BenchmarkResult{
		Mode:            mode,
		TestMode:        testMode,
		singleEmbedding: &out,
	}
}

// Helper: build BenchmarkResult from single completion report.
func SingleCompletionResult(mode, testMode string, report CompletionReport) BenchmarkResult {
	out := NewCompletionOutput(report, "")
	return BenchmarkResult{
		Mode:             mode,
		TestMode:         testMode,
		singleCompletion: &out,
	}
}

// Helper: build BenchmarkResult from single cache hit report.
func SingleCacheHitResult(report CacheHitReport) BenchmarkResult {
	out := NewCacheHitOutput(report)
	return BenchmarkResult{
		Mode:     ModeCompletion,
		TestMode: "cache_hit",
		cacheHit: &out,
	}
}

// Helper: build BenchmarkResult from PK embedding reports.
func PKEmbeddingResult(mode string, reportA, reportB EmbeddingReport, nameA, nameB string) BenchmarkResult {
	outA := NewEmbeddingOutput(reportA, nameA)
	outB := NewEmbeddingOutput(reportB, nameB)
	return BenchmarkResult{
		Mode:        mode,
		TestMode:    "pk",
		pkEmbedding: &EmbeddingPKOutput{
			ProviderA: outA,
			ProviderB: outB,
		},
	}
}

// Helper: build BenchmarkResult from PK completion reports.
func PKCompletionResult(mode string, reportA, reportB CompletionReport, nameA, nameB string) BenchmarkResult {
	outA := NewCompletionOutput(reportA, nameA)
	outB := NewCompletionOutput(reportB, nameB)
	return BenchmarkResult{
		Mode:     mode,
		TestMode: "pk",
		pkCompletion: &CompletionPKOutput{
			ProviderA: outA,
			ProviderB: outB,
		},
	}
}

// Helper: build BenchmarkResult from compare results.
func CompareResult(mode string, userMessage string, providerA, providerB CompareProviderResult) BenchmarkResult {
	validA := providerA.Error == ""
	validB := providerB.Error == ""
	return BenchmarkResult{
		Mode:     mode,
		TestMode: "compare",
		compare: &CompareOutput{
			UserMessage: userMessage,
			ProviderA:   providerA,
			ProviderB:   providerB,
			Valid:       validA && validB,
		},
	}
}

// Suppress unused import warning — time is used by CacheHitResultOutput.LatencyMs
var _ = time.Second
