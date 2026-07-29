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
	return maskCredential(key)
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

// --- completion single results ---

func mdWriteCompletionSingle(sb *strings.Builder, r *CompletionReport) {
	if r == nil {
		sb.WriteString("_No results available._\n")
		return
	}
	mdTable(sb, "Metric", "Value", mdSummaryRows(r.TotalRequests, r.SuccessCount, r.ErrorCount, r.ErrorCategories))
	if !r.Valid {
		sb.WriteString("_All requests failed — no metrics available._\n\n")
		mdWriteLogSection(sb, r)
		return
	}
	mdTable(sb, "Metric", "Value", [][]string{
		{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())},
		{"RPS", mdFmtF(r.RPS, 2)},
		{"Input TPS", mdFmtF(r.InputTPS, 1)},
		{"Output TPS", mdFmtF(r.OutputTPS, 1)},
		{"Input TPM", mdFmtF(r.InputTPM, 0)},
		{"Output TPM", mdFmtF(r.OutputTPM, 0)},
		{"Avg Output Tokens", mdFmtF(r.AvgOutputTokens, 1)},
	})
	mdTable(sb, "Metric", "Value", mdAPIUsageRows(r))
	mdTable(sb, "Metric", "Value", [][]string{
		{"TTFT Avg", mdFmtMs(r.TTFTAvg)},
		{"TTFT P50", mdFmtMs(r.TTFTp50)},
		{"TTFT P90", mdFmtMs(r.TTFTp90)},
		{"TTFT P99", mdFmtMs(r.TTFTp99)},
	})
	mdTable(sb, "Metric", "Value", [][]string{
		{"TPOT Avg", mdFmtMsPerTok(r.TPOTAvg)},
		{"TPOT P50", mdFmtMsPerTok(r.TPOTp50)},
		{"TPOT P90", mdFmtMsPerTok(r.TPOTp90)},
		{"TPOT P99", mdFmtMsPerTok(r.TPOTp99)},
	})
	e2e := [][]string{
		{"E2E Avg", mdFmtMs(r.E2EAvg)},
		{"E2E P50", mdFmtMs(r.E2Ep50)},
		{"E2E P90", mdFmtMs(r.E2Ep90)},
		{"E2E P99", mdFmtMs(r.E2Ep99)},
	}
	if r.SkippedChunks > 0 {
		e2e = append(e2e, []string{"Skipped SSE Chunks", fmt.Sprintf("%d (warning)", r.SkippedChunks)})
	}
	mdTable(sb, "Metric", "Value", e2e)
	if r.QueueTimeAvg > 0 || r.QueueTimeP50 > 0 || r.QueueTimeP90 > 0 || r.QueueTimeP99 > 0 {
		mdTable(sb, "Metric", "Value", [][]string{
			{"Queue Time Avg", mdFmtMs(r.QueueTimeAvg)},
			{"Queue Time P50", mdFmtMs(r.QueueTimeP50)},
			{"Queue Time P90", mdFmtMs(r.QueueTimeP90)},
			{"Queue Time P99", mdFmtMs(r.QueueTimeP99)},
		})
	}
	mdWriteLogSection(sb, r)
}

func mdAPIUsageRows(r *CompletionReport) [][]string {
	rows := [][]string{
		{"API Prompt Tokens", mdAPIUsageTokens(r, r.APIPromptTokens)},
		{"API Completion Tokens", mdAPIUsageTokens(r, r.APICompletionTokens)},
		{"API Total Tokens", mdAPIUsageTokens(r, r.APITotalTokens)},
		{"API Usage Samples", mdAPIUsageSamples(r)},
	}
	if r.MissingAPIUsageCount > 0 {
		rows = append(rows, []string{"Missing API Usage", fmt.Sprintf("%d", r.MissingAPIUsageCount)})
	}
	return rows
}

func mdAPIUsageTokens(r *CompletionReport, value int) string {
	if r.APIUsageCount == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d", value)
}

func mdAPIUsageSamples(r *CompletionReport) string {
	if r.SuccessCount == 0 {
		return "N/A"
	}
	if r.APIUsageCount == 0 {
		return fmt.Sprintf("N/A / %d", r.SuccessCount)
	}
	return fmt.Sprintf("%d / %d", r.APIUsageCount, r.SuccessCount)
}

func mdWriteLogSection(sb *strings.Builder, r *CompletionReport) {
	if r.LogRequestsFile == "" {
		return
	}
	sb.WriteString("## Logs\n\n")
	fmt.Fprintf(sb, "- Requests: `%s`\n", r.LogRequestsFile)
	fmt.Fprintf(sb, "- Responses: `%s`\n", r.LogResponsesFile)
	if r.LogDroppedCount > 0 {
		fmt.Fprintf(sb, "- ⚠ %d log entries dropped (disk too slow)\n", r.LogDroppedCount)
	}
	if r.LogError != "" {
		fmt.Fprintf(sb, "- ⚠ log write error: %s\n", r.LogError)
	}
}

// --- PK results ---

type mdPKRow struct {
	metric     string
	valA, valB string
	winner     int // 0 = A wins, 1 = B wins, -1 = tie/na
}

type mdMetricDef struct {
	metric         string
	valA, valB     float64
	format         func(float64) string
	higherIsBetter bool
}

func mdPKMetricRows(defs []mdMetricDef) []mdPKRow {
	rows := make([]mdPKRow, 0, len(defs))
	for _, d := range defs {
		rows = append(rows, mdPKRow{
			metric: d.metric,
			valA:   d.format(d.valA),
			valB:   d.format(d.valB),
			winner: mdWinner(d.valA, d.valB, d.higherIsBetter),
		})
	}
	return rows
}

func mdWinner(a, b float64, higherIsBetter bool) int {
	if a == b {
		return -1
	}
	if higherIsBetter {
		if a > b {
			return 0
		}
		return 1
	}
	if a < b {
		return 0
	}
	return 1
}

func mdWriteEmbeddingPK(sb *strings.Builder, providers []ProviderConfig, reports []*EmbeddingReport) {
	var a, b *EmbeddingReport
	if len(reports) > 0 {
		a = reports[0]
	}
	if len(reports) > 1 {
		b = reports[1]
	}
	nameA, nameB := mdPKNames(providers)
	aValid := a != nil && a.Valid
	bValid := b != nil && b.Valid

	rows := []mdPKRow{
		{metric: "Success / Total", valA: mdEmbeddingSuccessTotal(a), valB: mdEmbeddingSuccessTotal(b), winner: -1},
		{metric: "Error Categories", valA: mdSummarizeErrorCategories(embeddingCategories(a)), valB: mdSummarizeErrorCategories(embeddingCategories(b)), winner: -1},
	}
	if aValid && bValid {
		rows = append(rows, mdPKMetricRows([]mdMetricDef{
			{"RPS", a.RPS, b.RPS, func(v float64) string { return mdFmtF(v, 2) }, true},
			{"Input TPS", a.InputTPS, b.InputTPS, func(v float64) string { return mdFmtF(v, 1) }, true},
			{"Input TPM", a.InputTPM, b.InputTPM, func(v float64) string { return mdFmtF(v, 0) }, true},
			{"Latency Avg", a.LatencyAvg, b.LatencyAvg, mdFmtMs, false},
			{"Latency P50", a.LatencyP50, b.LatencyP50, mdFmtMs, false},
			{"Latency P90", a.LatencyP90, b.LatencyP90, mdFmtMs, false},
			{"Latency P99", a.LatencyP99, b.LatencyP99, mdFmtMs, false},
		})...)
	}
	mdPKTable(sb, nameA, nameB, rows)
	mdPKFailureNotes(sb, nameA, aValid, nameB, bValid)
}

func mdWriteCompletionPK(sb *strings.Builder, providers []ProviderConfig, reports []*CompletionReport) {
	var a, b *CompletionReport
	if len(reports) > 0 {
		a = reports[0]
	}
	if len(reports) > 1 {
		b = reports[1]
	}
	nameA, nameB := mdPKNames(providers)
	aValid := a != nil && a.Valid
	bValid := b != nil && b.Valid

	rows := []mdPKRow{
		{metric: "Success / Total", valA: mdCompletionSuccessTotal(a), valB: mdCompletionSuccessTotal(b), winner: -1},
		{metric: "Error Categories", valA: mdSummarizeErrorCategories(completionCategories(a)), valB: mdSummarizeErrorCategories(completionCategories(b)), winner: -1},
		{metric: "API Prompt Tokens", valA: mdPKAPIUsageTokens(a, func(r *CompletionReport) int { return r.APIPromptTokens }), valB: mdPKAPIUsageTokens(b, func(r *CompletionReport) int { return r.APIPromptTokens }), winner: -1},
		{metric: "API Completion Tokens", valA: mdPKAPIUsageTokens(a, func(r *CompletionReport) int { return r.APICompletionTokens }), valB: mdPKAPIUsageTokens(b, func(r *CompletionReport) int { return r.APICompletionTokens }), winner: -1},
		{metric: "API Total Tokens", valA: mdPKAPIUsageTokens(a, func(r *CompletionReport) int { return r.APITotalTokens }), valB: mdPKAPIUsageTokens(b, func(r *CompletionReport) int { return r.APITotalTokens }), winner: -1},
		{metric: "API Usage Samples", valA: mdPKAPIUsageSamples(a), valB: mdPKAPIUsageSamples(b), winner: -1},
	}
	if aValid && bValid {
		defs := []mdMetricDef{
			{"RPS", a.RPS, b.RPS, func(v float64) string { return mdFmtF(v, 2) }, true},
			{"Input TPS", a.InputTPS, b.InputTPS, func(v float64) string { return mdFmtF(v, 1) }, true},
			{"Output TPS", a.OutputTPS, b.OutputTPS, func(v float64) string { return mdFmtF(v, 1) }, true},
			{"Input TPM", a.InputTPM, b.InputTPM, func(v float64) string { return mdFmtF(v, 0) }, true},
			{"Output TPM", a.OutputTPM, b.OutputTPM, func(v float64) string { return mdFmtF(v, 0) }, true},
			{"TTFT Avg", a.TTFTAvg, b.TTFTAvg, mdFmtMs, false},
			{"TTFT P50", a.TTFTp50, b.TTFTp50, mdFmtMs, false},
			{"TTFT P90", a.TTFTp90, b.TTFTp90, mdFmtMs, false},
			{"TTFT P99", a.TTFTp99, b.TTFTp99, mdFmtMs, false},
			{"TPOT Avg", a.TPOTAvg, b.TPOTAvg, mdFmtMsPerTok, false},
			{"TPOT P50", a.TPOTp50, b.TPOTp50, mdFmtMsPerTok, false},
			{"TPOT P90", a.TPOTp90, b.TPOTp90, mdFmtMsPerTok, false},
			{"TPOT P99", a.TPOTp99, b.TPOTp99, mdFmtMsPerTok, false},
			{"E2E Avg", a.E2EAvg, b.E2EAvg, mdFmtMs, false},
			{"E2E P50", a.E2Ep50, b.E2Ep50, mdFmtMs, false},
			{"E2E P90", a.E2Ep90, b.E2Ep90, mdFmtMs, false},
			{"E2E P99", a.E2Ep99, b.E2Ep99, mdFmtMs, false},
		}
		if a.QueueTimeAvg > 0 || b.QueueTimeAvg > 0 {
			defs = append(defs,
				mdMetricDef{"Queue Time Avg", a.QueueTimeAvg, b.QueueTimeAvg, mdFmtMs, false},
				mdMetricDef{"Queue Time P50", a.QueueTimeP50, b.QueueTimeP50, mdFmtMs, false},
				mdMetricDef{"Queue Time P90", a.QueueTimeP90, b.QueueTimeP90, mdFmtMs, false},
				mdMetricDef{"Queue Time P99", a.QueueTimeP99, b.QueueTimeP99, mdFmtMs, false},
			)
		}
		rows = append(rows, mdPKMetricRows(defs)...)
	}
	mdPKTable(sb, nameA, nameB, rows)
	mdPKFailureNotes(sb, nameA, aValid, nameB, bValid)
}

func mdPKNames(providers []ProviderConfig) (string, string) {
	nameA, nameB := "Provider A", "Provider B"
	if len(providers) > 0 && providers[0].Name != "" {
		nameA = providers[0].Name
	}
	if len(providers) > 1 && providers[1].Name != "" {
		nameB = providers[1].Name
	}
	return nameA, nameB
}

func mdEmbeddingSuccessTotal(r *EmbeddingReport) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d / %d", r.SuccessCount, r.TotalRequests)
}

func mdCompletionSuccessTotal(r *CompletionReport) string {
	if r == nil {
		return ""
	}
	return fmt.Sprintf("%d / %d", r.SuccessCount, r.TotalRequests)
}

func embeddingCategories(r *EmbeddingReport) map[string]int {
	if r == nil {
		return nil
	}
	return r.ErrorCategories
}

func completionCategories(r *CompletionReport) map[string]int {
	if r == nil {
		return nil
	}
	return r.ErrorCategories
}

func mdPKAPIUsageTokens(r *CompletionReport, value func(*CompletionReport) int) string {
	if r == nil || r.APIUsageCount == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%d", value(r))
}

func mdPKAPIUsageSamples(r *CompletionReport) string {
	if r == nil || r.SuccessCount == 0 {
		return "N/A"
	}
	if r.APIUsageCount == 0 {
		return fmt.Sprintf("N/A / %d", r.SuccessCount)
	}
	return fmt.Sprintf("%d / %d", r.APIUsageCount, r.SuccessCount)
}

func mdPKTable(sb *strings.Builder, nameA, nameB string, rows []mdPKRow) {
	fmt.Fprintf(sb, "| Metric | %s | %s |\n", mdEscape(nameA), mdEscape(nameB))
	sb.WriteString("|---|---|---|\n")
	for _, r := range rows {
		va, vb := mdEscape(r.valA), mdEscape(r.valB)
		if r.winner == 0 {
			va = "**" + va + "**"
		}
		if r.winner == 1 {
			vb = "**" + vb + "**"
		}
		fmt.Fprintf(sb, "| %s | %s | %s |\n", mdEscape(r.metric), va, vb)
	}
	sb.WriteString("\n")
}

func mdPKFailureNotes(sb *strings.Builder, nameA string, aValid bool, nameB string, bValid bool) {
	if !aValid {
		fmt.Fprintf(sb, "_%s: all requests failed_\n", nameA)
	}
	if !bValid {
		fmt.Fprintf(sb, "_%s: all requests failed_\n", nameB)
	}
}
