package bench

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MarkdownBenchReport renders a benchmark report (embedding or completion-like,
// single or PK) as a markdown document. Exactly one of embReports / compReports
// is non-empty.
func MarkdownBenchReport(apiMode, testMode string, providers []ProviderConfig, cfg BenchConfig, embReports []*EmbeddingReport, compReports []*CompletionReport, now time.Time) string {
	var sb strings.Builder
	title := "# Benchmark Report — " + mdModeDisplayName(apiMode)
	if testMode == "pk" {
		title += " (PK)"
	}
	sb.WriteString(title + "\n\n")
	sb.WriteString("Generated: " + now.Format("2006-01-02 15:04:05") + "\n\n")

	sb.WriteString("## Test Parameters\n\n")
	mdWriteBenchParams(&sb, apiMode, testMode, providers, cfg)

	sb.WriteString("## Results\n\n")
	if apiMode == ModeEmbedding {
		if testMode == "pk" {
			mdWriteEmbeddingPK(&sb, providers, embReports)
		} else {
			mdWriteEmbeddingSingle(&sb, firstEmbeddingReport(embReports))
		}
	} else {
		if testMode == "pk" {
			mdWriteCompletionPK(&sb, providers, compReports)
		} else {
			mdWriteCompletionSingle(&sb, firstCompletionReport(compReports))
		}
	}
	return sb.String()
}

func firstEmbeddingReport(reports []*EmbeddingReport) *EmbeddingReport {
	if len(reports) == 0 {
		return nil
	}
	return reports[0]
}

func firstCompletionReport(reports []*CompletionReport) *CompletionReport {
	if len(reports) == 0 {
		return nil
	}
	return reports[0]
}

func mdModeDisplayName(apiMode string) string {
	switch apiMode {
	case ModeEmbedding:
		return "Embedding"
	case ModeAnthropicMessages:
		return "Anthropic Messages"
	default:
		return "Chat Completion"
	}
}

// --- parameters ---

func mdWriteBenchParams(sb *strings.Builder, apiMode, testMode string, providers []ProviderConfig, cfg BenchConfig) {
	if testMode == "pk" && len(providers) >= 2 {
		rows := [][3]string{
			{"URL", providers[0].URL, providers[1].URL},
			{"Model", providers[0].Model, providers[1].Model},
			{"API Key", mdRedactAPIKey(providers[0].APIKey), mdRedactAPIKey(providers[1].APIKey)},
		}
		if providers[0].CustomParams != "" || providers[1].CustomParams != "" {
			rows = append(rows, [3]string{"Custom Params", providers[0].CustomParams, providers[1].CustomParams})
		}
		mdTable3(sb, "Parameter", providers[0].Name, providers[1].Name, rows)
		mdTable(sb, "Parameter", "Value", mdSharedBenchParams(apiMode, testMode, cfg))
		return
	}

	var rows [][]string
	if len(providers) >= 1 {
		p := providers[0]
		rows = append(rows,
			[]string{"Provider", p.Name},
			[]string{"URL", p.URL},
			[]string{"Model", p.Model},
			[]string{"API Key", mdRedactAPIKey(p.APIKey)},
		)
		if p.CustomParams != "" {
			rows = append(rows, []string{"Custom Params", p.CustomParams})
		}
	}
	rows = append(rows, mdSharedBenchParams(apiMode, testMode, cfg)...)
	mdTable(sb, "Parameter", "Value", rows)
}

func mdSharedBenchParams(apiMode, testMode string, cfg BenchConfig) [][]string {
	isCompletionLike := apiMode == ModeCompletion || apiMode == ModeAnthropicMessages
	rows := [][]string{
		{"API Mode", apiMode},
		{"Test Mode", testMode},
	}
	if cfg.LoadModel == LoadModelOpenLoop {
		rows = append(rows,
			[]string{"Load Model", cfg.LoadModel},
			[]string{"Request Rate", fmt.Sprintf("%d req/s", cfg.RequestRate)},
			[]string{"Max In-Flight", fmt.Sprintf("%d", cfg.MaxInFlight)},
		)
	} else {
		rows = append(rows, []string{"Concurrency", fmt.Sprintf("%d", cfg.Concurrency)})
		if apiMode == ModeCompletion {
			rows = append(rows, []string{"Load Model", LoadModelClosedLoop})
		}
	}
	rows = append(rows,
		[]string{"Total Requests", fmt.Sprintf("%d", cfg.TotalRequests)},
		[]string{"Input Tokens", fmt.Sprintf("%d", cfg.TargetTokens)},
	)
	if isCompletionLike {
		rows = append(rows, []string{"Max Output Tokens", mdMaxOutputTokens(cfg.MaxOutputTokens)})
		rows = append(rows, []string{"TTFT Includes Reasoning", strconv.FormatBool(cfg.TTFTIncludesReasoning)})
		if cfg.SystemPrompt != "" {
			rows = append(rows, []string{"System Prompt", cfg.SystemPrompt})
		}
	}
	if apiMode == ModeCompletion && testMode == "single" {
		rows = append(rows, []string{"Request Logging", strconv.FormatBool(cfg.RequestLogging)})
	}
	return rows
}

func mdMaxOutputTokens(n int) string {
	if n <= 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", n)
}

// --- embedding single results ---

func mdWriteEmbeddingSingle(sb *strings.Builder, r *EmbeddingReport) {
	if r == nil {
		sb.WriteString("_No results available._\n")
		return
	}
	mdTable(sb, "Metric", "Value", mdSummaryRows(r.TotalRequests, r.SuccessCount, r.ErrorCount, r.ErrorCategories))
	if !r.Valid {
		sb.WriteString("_All requests failed — no metrics available._\n")
		return
	}
	mdTable(sb, "Metric", "Value", [][]string{
		{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())},
		{"RPS", mdFmtF(r.RPS, 2)},
		{"Input TPS", mdFmtF(r.InputTPS, 1)},
		{"Input TPM", mdFmtF(r.InputTPM, 0)},
		{"Latency Avg", mdFmtMs(r.LatencyAvg)},
		{"Latency P50", mdFmtMs(r.LatencyP50)},
		{"Latency P90", mdFmtMs(r.LatencyP90)},
		{"Latency P99", mdFmtMs(r.LatencyP99)},
	})
}

func mdSummaryRows(total, success, failed int, categories map[string]int) [][]string {
	successPct := 0.0
	if total > 0 {
		successPct = float64(success) / float64(total) * 100
	}
	rows := [][]string{
		{"Total Requests", fmt.Sprintf("%d", total)},
		{"Successful", fmt.Sprintf("%d (%.1f%%)", success, successPct)},
		{"Failed", fmt.Sprintf("%d", failed)},
	}
	if summary := mdSummarizeErrorCategories(categories); summary != "" {
		rows = append(rows, []string{"Error Categories", summary})
	}
	return rows
}

// --- helpers ---

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}

func mdRedactAPIKey(key string) string {
	if key == "" {
		return "(none)"
	}
	if len(key) <= 4 {
		return "***"
	}
	return "***" + key[len(key)-4:]
}

func mdTable(sb *strings.Builder, col1, col2 string, rows [][]string) {
	fmt.Fprintf(sb, "| %s | %s |\n|---|---|\n", col1, col2)
	for _, r := range rows {
		if len(r) < 2 {
			continue
		}
		fmt.Fprintf(sb, "| %s | %s |\n", mdEscape(r[0]), mdEscape(r[1]))
	}
	sb.WriteString("\n")
}

func mdTable3(sb *strings.Builder, col1, col2, col3 string, rows [][3]string) {
	fmt.Fprintf(sb, "| %s | %s | %s |\n|---|---|---|\n", mdEscape(col1), mdEscape(col2), mdEscape(col3))
	for _, r := range rows {
		fmt.Fprintf(sb, "| %s | %s | %s |\n", mdEscape(r[0]), mdEscape(r[1]), mdEscape(r[2]))
	}
	sb.WriteString("\n")
}

func mdFmtF(v float64, decimals int) string {
	return fmt.Sprintf(fmt.Sprintf("%%.%df", decimals), v)
}

func mdFmtMs(v float64) string { return fmt.Sprintf("%.2f ms", v) }

func mdFmtMsPerTok(v float64) string { return fmt.Sprintf("%.2f ms/tok", v) }

func mdSummarizeErrorCategories(categories map[string]int) string {
	if len(categories) == 0 {
		return ""
	}
	type kv struct {
		name  string
		count int
	}
	pairs := make([]kv, 0, len(categories))
	for name, count := range categories {
		if count > 0 {
			pairs = append(pairs, kv{name, count})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].name < pairs[j].name
	})
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s: %d", p.name, p.count))
	}
	return strings.Join(parts, ", ")
}

// Temporary stubs — implemented in later tasks.
func mdWriteEmbeddingPK(sb *strings.Builder, providers []ProviderConfig, reports []*EmbeddingReport) {
}
func mdWriteCompletionSingle(sb *strings.Builder, r *CompletionReport) {}
func mdWriteCompletionPK(sb *strings.Builder, providers []ProviderConfig, reports []*CompletionReport) {
}
