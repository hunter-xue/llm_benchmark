package tui

import (
	"fmt"
	"strings"

	"embedding_benchmark/internal/bench"
)

type cacheHitResultsModel struct {
	report *bench.CacheHitReport
	width  int
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

func (m cacheHitResultsModel) view(width, height int) string {
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

	renderTwoColTable(&sb, [][]string{
		{"Total Requests", fmt.Sprintf("%d", r.TotalRequests)},
		{"Successful", fmt.Sprintf("%d (%.1f%%)", r.SuccessCount, successPct)},
		{"Failed", fmt.Sprintf("%d", r.ErrorCount)},
		{"Wall Time", fmt.Sprintf("%.2f s", r.WallTime.Seconds())},
	})

	if !r.Valid {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render("  All requests failed — no cache metrics available."))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  Press 'e' to view error details."))
		sb.WriteString("\n")
		sb.WriteString("\n")
		sb.WriteString(helpStyle.Render("r rerun  •  e errors  •  ctrl+e export  •  esc config  •  ctrl+c quit"))
		return sb.String()
	}

	sb.WriteString(dimStyle.Render("  " + strings.Repeat("-", 48)))
	sb.WriteString("\n")
	renderTwoColTable(&sb, [][]string{
		{"Prompt Tokens", fmt.Sprintf("%d", r.TotalPromptTokens)},
		{"Cached Tokens", fmt.Sprintf("%d", r.TotalCachedTokens)},
		{"Overall Hit Rate", fmt.Sprintf("%.2f%%", r.CacheHitRate)},
		{"Avg Request Hit Rate", fmt.Sprintf("%.2f%%", r.AvgRequestHitRate)},
		{"Avg Latency", fmtMs(r.AvgLatencyMs)},
	})
	if r.MissingUsageCount > 0 || r.MissingCachedCount > 0 {
		renderTwoColTable(&sb, [][]string{
			{"Missing Usage", fmt.Sprintf("%d", r.MissingUsageCount)},
			{"Missing Cached Field", fmt.Sprintf("%d", r.MissingCachedCount)},
		})
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  #    Latency       Prompt    Cached    Hit Rate"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  " + strings.Repeat("-", 48)))
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
			if result.PromptTokens > 0 {
				hitRate = fmt.Sprintf("%.2f%%", float64(result.CachedTokens)/float64(result.PromptTokens)*100)
			}
		}
		sb.WriteString(fmt.Sprintf("  %-4d %-12s %-9d %-9s %s\n",
			result.Index,
			fmtMs(float64(result.Latency.Milliseconds())),
			result.PromptTokens,
			cached,
			hitRate,
		))
	}

	sb.WriteString("\n")
	hints := "r rerun  •  ctrl+e export  •  esc config  •  ctrl+c quit"
	if m.hasErrors() {
		hints = "r rerun  •  e errors  •  ctrl+e export  •  esc config  •  ctrl+c quit"
	}
	sb.WriteString(helpStyle.Render(hints))
	return sb.String()
}
