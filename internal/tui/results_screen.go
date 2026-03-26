package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"embedding_benchmark/internal/bench"
)

type resultsModel struct {
	apiMode          string
	testMode         string
	embeddingReports []*bench.EmbeddingReport  // len 1 for single, 2 for PK
	completionReports []*bench.CompletionReport // len 1 for single, 2 for PK
	providerNames    []string
	hasErrors        bool
	width            int
}

func newResultsModel(apiMode, testMode string, providerNames []string) resultsModel {
	return resultsModel{
		apiMode:       apiMode,
		testMode:      testMode,
		providerNames: providerNames,
	}
}

func (m *resultsModel) addEmbeddingResult(idx int, r *bench.EmbeddingReport) {
	if idx == 0 {
		m.embeddingReports = []*bench.EmbeddingReport{r, nil}
	} else if idx == 1 && len(m.embeddingReports) > 0 {
		m.embeddingReports[1] = r
	}
	if r != nil && len(r.ErrorDetails) > 0 {
		m.hasErrors = true
	}
}

func (m *resultsModel) addCompletionResult(idx int, r *bench.CompletionReport) {
	if idx == 0 {
		m.completionReports = []*bench.CompletionReport{r, nil}
	} else if idx == 1 && len(m.completionReports) > 0 {
		m.completionReports[1] = r
	}
	if r != nil && len(r.ErrorDetails) > 0 {
		m.hasErrors = true
	}
}

func (m resultsModel) mergedErrorDetails() map[string]int {
	merged := make(map[string]int)
	for _, r := range m.embeddingReports {
		if r != nil {
			for k, v := range r.ErrorDetails {
				merged[k] += v
			}
		}
	}
	for _, r := range m.completionReports {
		if r != nil {
			for k, v := range r.ErrorDetails {
				merged[k] += v
			}
		}
	}
	return merged
}

func (m resultsModel) update(msg tea.Msg) (resultsModel, tea.Cmd) {
	return m, nil
}

func (m *resultsModel) setWidth(w int) {
	m.width = w
}

type pkRow struct {
	metric        string
	valA          string
	valB          string
	winnerIdx     int  // 0=A, 1=B, -1=tie/na
	higherIsBetter bool
}

func (m resultsModel) view(width, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Benchmark Results"))
	sb.WriteString("\n\n")

	isPK := m.testMode == "pk"

	if m.apiMode == bench.ModeEmbedding {
		if isPK {
			m.renderEmbeddingPK(&sb)
		} else {
			m.renderEmbeddingSingle(&sb)
		}
	} else {
		if isPK {
			m.renderCompletionPK(&sb)
		} else {
			m.renderCompletionSingle(&sb)
		}
	}

	sb.WriteString("\n")
	hints := "r rerun  •  esc back  •  ctrl+c quit"
	if m.hasErrors {
		hints = "r rerun  •  e view errors  •  esc back  •  ctrl+c quit"
	}
	sb.WriteString(helpStyle.Render(hints))
	return sb.String()
}

func fmtF(v float64, decimals int) string {
	return fmt.Sprintf(fmt.Sprintf("%%.%df", decimals), v)
}

func fmtMs(v float64) string {
	return fmt.Sprintf("%.2f ms", v)
}

func fmtMsPerTok(v float64) string {
	return fmt.Sprintf("%.2f ms/tok", v)
}

func (m resultsModel) renderEmbeddingSingle(sb *strings.Builder) {
	if len(m.embeddingReports) == 0 || m.embeddingReports[0] == nil {
		sb.WriteString(errorStyle.Render("No results available."))
		return
	}
	r := m.embeddingReports[0]

	successPct := 0.0
	if r.TotalRequests > 0 {
		successPct = float64(r.SuccessCount) / float64(r.TotalRequests) * 100
	}

	summaryRows := [][]string{
		{"Total Requests", fmt.Sprintf("%d", r.TotalRequests)},
		{"Successful", fmt.Sprintf("%d (%.1f%%)", r.SuccessCount, successPct)},
		{"Failed", fmt.Sprintf("%d", r.ErrorCount)},
	}
	renderTwoColTable(sb, summaryRows)

	if !r.Valid {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render("  All requests failed — no metrics available."))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  Press 'e' to view error details."))
		sb.WriteString("\n")
		return
	}

	metricRows := [][]string{
		{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())},
		{"RPS", fmtF(r.RPS, 2)},
		{"Input TPS", fmtF(r.InputTPS, 1)},
		{"Input TPM", fmtF(r.InputTPM, 0)},
		{"Latency Avg", fmtMs(r.LatencyAvg)},
		{"Latency P50", fmtMs(r.LatencyP50)},
		{"Latency P90", fmtMs(r.LatencyP90)},
		{"Latency P99", fmtMs(r.LatencyP99)},
	}
	renderTwoColTable(sb, metricRows)

	if r.ErrorCount > 0 {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  %d request(s) failed.", r.ErrorCount)))
		sb.WriteString("  ")
		sb.WriteString(dimStyle.Render("Press 'e' to view error details."))
		sb.WriteString("\n")
	}
}

func (m resultsModel) renderEmbeddingPK(sb *strings.Builder) {
	if len(m.embeddingReports) < 2 {
		sb.WriteString(errorStyle.Render("Waiting for both results..."))
		return
	}
	a := m.embeddingReports[0]
	b := m.embeddingReports[1]
	nameA := "Provider A"
	nameB := "Provider B"
	if len(m.providerNames) >= 2 {
		nameA = m.providerNames[0]
		nameB = m.providerNames[1]
	}

	aValid := a != nil && a.Valid
	bValid := b != nil && b.Valid

	getA := func(v float64, f func(float64) string) string {
		if !aValid {
			return naStyle.Render("N/A")
		}
		return f(v)
	}
	getB := func(v float64, f func(float64) string) string {
		if !bValid {
			return naStyle.Render("N/A")
		}
		return f(v)
	}

	type row struct {
		metric         string
		valA, valB     float64
		fmtFn          func(float64) string
		higherIsBetter bool
	}

	rows := []row{}
	if aValid && bValid {
		rows = []row{
			{"RPS", a.RPS, b.RPS, func(v float64) string { return fmtF(v, 2) }, true},
			{"Input TPS", a.InputTPS, b.InputTPS, func(v float64) string { return fmtF(v, 1) }, true},
			{"Input TPM", a.InputTPM, b.InputTPM, func(v float64) string { return fmtF(v, 0) }, true},
			{"Latency Avg", a.LatencyAvg, b.LatencyAvg, fmtMs, false},
			{"Latency P50", a.LatencyP50, b.LatencyP50, fmtMs, false},
			{"Latency P90", a.LatencyP90, b.LatencyP90, fmtMs, false},
			{"Latency P99", a.LatencyP99, b.LatencyP99, fmtMs, false},
		}
	}

	// Success/fail rows (always shown)
	renderPKHeader(sb, nameA, nameB)

	successA := ""
	successB := ""
	if a != nil {
		successA = fmt.Sprintf("%d / %d", a.SuccessCount, a.TotalRequests)
	}
	if b != nil {
		successB = fmt.Sprintf("%d / %d", b.SuccessCount, b.TotalRequests)
	}
	renderPKRow(sb, "Success / Total", successA, successB, -1)

	for _, r := range rows {
		va := getA(r.valA, r.fmtFn)
		vb := getB(r.valB, r.fmtFn)
		winner := -1
		if aValid && bValid {
			if r.higherIsBetter {
				if r.valA > r.valB {
					winner = 0
				} else if r.valB > r.valA {
					winner = 1
				}
			} else {
				if r.valA < r.valB {
					winner = 0
				} else if r.valB < r.valA {
					winner = 1
				}
			}
		}
		renderPKRow(sb, r.metric, va, vb, winner)
	}

	if !aValid {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  %s: all requests failed", nameA)))
	}
	if !bValid {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  %s: all requests failed", nameB)))
	}
}

func (m resultsModel) renderCompletionSingle(sb *strings.Builder) {
	if len(m.completionReports) == 0 || m.completionReports[0] == nil {
		sb.WriteString(errorStyle.Render("No results available."))
		return
	}
	r := m.completionReports[0]

	successPct := 0.0
	if r.TotalRequests > 0 {
		successPct = float64(r.SuccessCount) / float64(r.TotalRequests) * 100
	}

	summaryRows := [][]string{
		{"Total Requests", fmt.Sprintf("%d", r.TotalRequests)},
		{"Successful", fmt.Sprintf("%d (%.1f%%)", r.SuccessCount, successPct)},
		{"Failed", fmt.Sprintf("%d", r.ErrorCount)},
	}
	renderTwoColTable(sb, summaryRows)

	if !r.Valid {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render("  All requests failed — no metrics available."))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  Press 'e' to view error details."))
		sb.WriteString("\n")
		return
	}

	renderTwoColTable(sb, [][]string{
		{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())},
		{"RPS", fmtF(r.RPS, 2)},
		{"Input TPS", fmtF(r.InputTPS, 1)},
		{"Output TPS", fmtF(r.OutputTPS, 1)},
		{"Input TPM", fmtF(r.InputTPM, 0)},
		{"Output TPM", fmtF(r.OutputTPM, 0)},
		{"Avg Output Tokens", fmtF(r.AvgOutputTokens, 1)},
	})
	sb.WriteString(dimStyle.Render("  " + strings.Repeat("─", 36)))
	sb.WriteString("\n")
	renderTwoColTable(sb, [][]string{
		{"TTFT Avg", fmtMs(r.TTFTAvg)},
		{"TTFT P50", fmtMs(r.TTFTp50)},
		{"TTFT P90", fmtMs(r.TTFTp90)},
		{"TTFT P99", fmtMs(r.TTFTp99)},
	})
	sb.WriteString(dimStyle.Render("  " + strings.Repeat("─", 36)))
	sb.WriteString("\n")
	renderTwoColTable(sb, [][]string{
		{"TPOT Avg", fmtMsPerTok(r.TPOTAvg)},
		{"TPOT P50", fmtMsPerTok(r.TPOTp50)},
		{"TPOT P90", fmtMsPerTok(r.TPOTp90)},
		{"TPOT P99", fmtMsPerTok(r.TPOTp99)},
	})
	sb.WriteString(dimStyle.Render("  " + strings.Repeat("─", 36)))
	sb.WriteString("\n")
	e2eRows := [][]string{
		{"E2E Avg", fmtMs(r.E2EAvg)},
		{"E2E P50", fmtMs(r.E2Ep50)},
		{"E2E P90", fmtMs(r.E2Ep90)},
		{"E2E P99", fmtMs(r.E2Ep99)},
	}
	if r.SkippedChunks > 0 {
		e2eRows = append(e2eRows, []string{"Skipped SSE Chunks", fmt.Sprintf("%d (warning)", r.SkippedChunks)})
	}
	renderTwoColTable(sb, e2eRows)

	if r.ErrorCount > 0 {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  %d request(s) failed.", r.ErrorCount)))
		sb.WriteString("  ")
		sb.WriteString(dimStyle.Render("Press 'e' to view error details."))
		sb.WriteString("\n")
	}
}

func (m resultsModel) renderCompletionPK(sb *strings.Builder) {
	if len(m.completionReports) < 2 {
		sb.WriteString(errorStyle.Render("Waiting for both results..."))
		return
	}
	a := m.completionReports[0]
	b := m.completionReports[1]
	nameA := "Provider A"
	nameB := "Provider B"
	if len(m.providerNames) >= 2 {
		nameA = m.providerNames[0]
		nameB = m.providerNames[1]
	}

	aValid := a != nil && a.Valid
	bValid := b != nil && b.Valid

	type row struct {
		metric         string
		valA, valB     float64
		fmtFn          func(float64) string
		higherIsBetter bool
		sep            bool // if true, render a separator line instead of a data row
	}

	var compRows []row
	if aValid && bValid {
		compRows = []row{
			{"RPS", a.RPS, b.RPS, func(v float64) string { return fmtF(v, 2) }, true, false},
			{"Input TPS", a.InputTPS, b.InputTPS, func(v float64) string { return fmtF(v, 1) }, true, false},
			{"Output TPS", a.OutputTPS, b.OutputTPS, func(v float64) string { return fmtF(v, 1) }, true, false},
			{"Input TPM", a.InputTPM, b.InputTPM, func(v float64) string { return fmtF(v, 0) }, true, false},
			{"Output TPM", a.OutputTPM, b.OutputTPM, func(v float64) string { return fmtF(v, 0) }, true, false},
			{"", 0, 0, nil, false, true},
			{"TTFT Avg", a.TTFTAvg, b.TTFTAvg, fmtMs, false, false},
			{"TTFT P50", a.TTFTp50, b.TTFTp50, fmtMs, false, false},
			{"TTFT P90", a.TTFTp90, b.TTFTp90, fmtMs, false, false},
			{"TTFT P99", a.TTFTp99, b.TTFTp99, fmtMs, false, false},
			{"", 0, 0, nil, false, true},
			{"TPOT Avg", a.TPOTAvg, b.TPOTAvg, fmtMsPerTok, false, false},
			{"TPOT P50", a.TPOTp50, b.TPOTp50, fmtMsPerTok, false, false},
			{"TPOT P90", a.TPOTp90, b.TPOTp90, fmtMsPerTok, false, false},
			{"TPOT P99", a.TPOTp99, b.TPOTp99, fmtMsPerTok, false, false},
			{"", 0, 0, nil, false, true},
			{"E2E Avg", a.E2EAvg, b.E2EAvg, fmtMs, false, false},
			{"E2E P50", a.E2Ep50, b.E2Ep50, fmtMs, false, false},
			{"E2E P90", a.E2Ep90, b.E2Ep90, fmtMs, false, false},
			{"E2E P99", a.E2Ep99, b.E2Ep99, fmtMs, false, false},
		}
	}

	renderPKHeader(sb, nameA, nameB)

	successA, successB := "", ""
	if a != nil {
		successA = fmt.Sprintf("%d / %d", a.SuccessCount, a.TotalRequests)
	}
	if b != nil {
		successB = fmt.Sprintf("%d / %d", b.SuccessCount, b.TotalRequests)
	}
	renderPKRow(sb, "Success / Total", successA, successB, -1)

	for _, r := range compRows {
		if r.sep {
			renderPKSeparator(sb)
			continue
		}
		va := r.fmtFn(r.valA)
		vb := r.fmtFn(r.valB)
		if !aValid {
			va = naStyle.Render("N/A")
		}
		if !bValid {
			vb = naStyle.Render("N/A")
		}
		winner := -1
		if aValid && bValid {
			if r.higherIsBetter {
				if r.valA > r.valB {
					winner = 0
				} else if r.valB > r.valA {
					winner = 1
				}
			} else {
				if r.valA < r.valB {
					winner = 0
				} else if r.valB < r.valA {
					winner = 1
				}
			}
		}
		renderPKRow(sb, r.metric, va, vb, winner)
	}

	if !aValid {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  %s: all requests failed", nameA)))
	}
	if !bValid {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render(fmt.Sprintf("  %s: all requests failed", nameB)))
	}
}

func renderPKSeparator(sb *strings.Builder) {
	sb.WriteString(dimStyle.Render("  " + strings.Repeat("─", 68)))
	sb.WriteString("\n")
}

func renderPKHeader(sb *strings.Builder, nameA, nameB string) {
	header := fmt.Sprintf("  %-22s  %-22s  %-22s", "Metric", nameA, nameB)
	sb.WriteString(dimStyle.Render(header))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  " + strings.Repeat("-", 68)))
	sb.WriteString("\n")
}

func renderPKRow(sb *strings.Builder, metric, valA, valB string, winnerIdx int) {
	renderVal := func(v string, isWinner bool) string {
		// Strip any existing ANSI before re-styling
		if isWinner {
			return winnerStyle.Width(22).Render(v)
		}
		return dimStyle.Width(22).Render(v)
	}
	aStr := renderVal(valA, winnerIdx == 0)
	bStr := renderVal(valB, winnerIdx == 1)
	sb.WriteString(fmt.Sprintf("  %-22s  %s  %s\n", metric, aStr, bStr))
}

func renderTwoColTable(sb *strings.Builder, rows [][]string) {
	for _, row := range rows {
		if len(row) < 2 {
			continue
		}
		label := labelStyle.Render(row[0] + ":")
		sb.WriteString(fmt.Sprintf("  %s  %s\n", label, row[1]))
	}
}
