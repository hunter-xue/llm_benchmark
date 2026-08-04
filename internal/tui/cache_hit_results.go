package tui

import (
	"fmt"
	"strings"

	"embedding_benchmark/internal/bench"
)

type cacheHitResultsModel struct {
	report     *bench.CacheHitReport
	width      int
	reportFile string // auto-written markdown report file name ("" = not written)
	reportErr  string // markdown report write error, if any
}

func newCacheHitResultsModel() cacheHitResultsModel {
	return cacheHitResultsModel{}
}

func (m *cacheHitResultsModel) setWidth(w int) {
	m.width = w
}

func (m *cacheHitResultsModel) setReport(r *bench.CacheHitReport) {
	m.report = r
}

func (m cacheHitResultsModel) hasErrors() bool {
	return m.report != nil && len(m.report.ErrorDetails) > 0
}

func (m cacheHitResultsModel) errorDetails() map[string]int {
	if m.report == nil {
		return nil
	}
	return m.report.ErrorDetails
}

func (m cacheHitResultsModel) errorCategories() map[string]int {
	if m.report == nil {
		return nil
	}
	return m.report.ErrorCategories
}

func (m cacheHitResultsModel) isAnthropic() bool {
	return m.report != nil && m.report.APIMode == bench.ModeAnthropicMessages
}

func (m cacheHitResultsModel) view(width, height int) string {
	return m.render(true)
}

// plainText returns the view without the report status line (used by ctrl+e export).
func (m cacheHitResultsModel) plainText() string {
	return m.render(false)
}

func (m cacheHitResultsModel) render(withStatus bool) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Prompt Cache Hit Results"))
	sb.WriteString("\n\n")
	if m.report == nil {
		sb.WriteString(errorStyle.Render("No results available."))
		return sb.String()
	}
	r := m.report
	successPct := 0.0
	if r.TotalRequests > 0 {
		successPct = float64(r.SuccessCount) / float64(r.TotalRequests) * 100
	}
	isAnthropic := m.isAnthropic()

	renderTwoColTable(&sb, [][]string{
		{"Total Requests", fmt.Sprintf("%d", r.TotalRequests)},
		{"Successful", fmt.Sprintf("%d (%.1f%%)", r.SuccessCount, successPct)},
		{"Failed", fmt.Sprintf("%d", r.ErrorCount)},
		{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())},
	})
	renderErrorCategorySummary(&sb, r.ErrorCategories)

	if !r.Valid {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render("  All requests failed — no cache metrics available."))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  Press 'e' to view error details."))
		sb.WriteString("\n")
		sb.WriteString("\n")
		if withStatus {
			renderReportStatus(&sb, m.reportFile, m.reportErr)
		}
		sb.WriteString(helpStyle.Render("r rerun  •  e errors  •  ctrl+e export  •  esc config  •  ctrl+c quit"))
		return sb.String()
	}

	sb.WriteString(dimStyle.Render("  " + strings.Repeat("-", 48)))
	sb.WriteString("\n")
	inputLabel := "Prompt Tokens"
	cachedLabel := "Cached Tokens"
	if isAnthropic {
		inputLabel = "Input Tokens"
		cachedLabel = "Cache Read Tokens"
	}
	summaryRows := [][]string{
		{inputLabel, fmt.Sprintf("%d", r.TotalPromptTokens)},
		{cachedLabel, fmt.Sprintf("%d", r.TotalCachedTokens)},
	}
	if isAnthropic {
		summaryRows = append(summaryRows, []string{"Cache Creation Tokens", fmt.Sprintf("%d", r.TotalCacheCreationTokens)})
	}
	summaryRows = append(summaryRows,
		[]string{"Overall Hit Rate", fmt.Sprintf("%.2f%%", r.CacheHitRate)},
		[]string{"Avg Request Hit Rate", fmt.Sprintf("%.2f%%", r.AvgRequestHitRate)},
		[]string{"Avg Latency", fmtMs(r.AvgLatencyMs)},
	)
	renderTwoColTable(&sb, summaryRows)
	if r.MissingUsageCount > 0 || r.MissingCachedCount > 0 {
		renderTwoColTable(&sb, [][]string{
			{"Missing Usage", fmt.Sprintf("%d", r.MissingUsageCount)},
			{"Missing Cached Field", fmt.Sprintf("%d", r.MissingCachedCount)},
		})
	}

	sb.WriteString("\n")
	if isAnthropic {
		sb.WriteString(dimStyle.Render("  #    Latency       Input     CacheRead Creation  Hit Rate"))
	} else {
		sb.WriteString(dimStyle.Render("  #    Latency       Prompt    Cached    Hit Rate"))
	}
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  " + strings.Repeat("-", 56)))
	sb.WriteString("\n")
	for _, result := range r.Results {
		if result.Err != nil {
			sb.WriteString(fmt.Sprintf("  %-4d %s\n", result.Index, errorStyle.Render(result.Err.Error())))
			continue
		}
		cached := "N/A"
		hitRate := "N/A"
		if result.HasCachedTokens {
			cached = fmt.Sprintf("%d", result.CachedTokens)
			denom := result.PromptTokens
			if isAnthropic {
				denom = result.PromptTokens + result.CacheCreationTokens + result.CachedTokens
			}
			if denom > 0 {
				hitRate = fmt.Sprintf("%.2f%%", float64(result.CachedTokens)/float64(denom)*100)
			}
		}
		if isAnthropic {
			creation := "N/A"
			if result.HasCacheCreation {
				creation = fmt.Sprintf("%d", result.CacheCreationTokens)
			}
			sb.WriteString(fmt.Sprintf("  %-4d %-12s %-9d %-9s %-9s %s\n",
				result.Index,
				fmtMs(float64(result.Latency.Milliseconds())),
				result.PromptTokens,
				cached,
				creation,
				hitRate,
			))
		} else {
			sb.WriteString(fmt.Sprintf("  %-4d %-12s %-9d %-9s %s\n",
				result.Index,
				fmtMs(float64(result.Latency.Milliseconds())),
				result.PromptTokens,
				cached,
				hitRate,
			))
		}
	}

	sb.WriteString("\n")
	if withStatus {
		renderReportStatus(&sb, m.reportFile, m.reportErr)
	}
	hints := "r rerun  •  ctrl+e export  •  esc config  •  ctrl+c quit"
	if m.hasErrors() {
		hints = "r rerun  •  e errors  •  ctrl+e export  •  esc config  •  ctrl+c quit"
	}
	sb.WriteString(helpStyle.Render(hints))
	return sb.String()
}
