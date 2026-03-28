package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type responseCompareModel struct {
	vpA, vpB      viewport.Model
	nameA, nameB  string
	focused       int // 0 = left (A), 1 = right (B)
	ready         bool
	width, height int
	loadingA, loadingB bool
	spinner       spinner.Model
	plainA, plainB string // uncolored text for export
}

func newResponseCompare(nameA, nameB string) responseCompareModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return responseCompareModel{
		nameA:    nameA,
		nameB:    nameB,
		loadingA: true,
		loadingB: true,
		spinner:  s,
	}
}

func (m *responseCompareModel) setSize(w, h int) {
	m.width = w
	m.height = h

	// Each panel: half width minus border/padding, height minus header+footer
	panelW := w/2 - 4
	panelH := h - 6
	if panelW < 10 {
		panelW = 10
	}
	if panelH < 3 {
		panelH = 3
	}

	if !m.ready {
		m.vpA = viewport.New(panelW, panelH)
		m.vpB = viewport.New(panelW, panelH)
		m.vpA.SetContent("  Loading...")
		m.vpB.SetContent("  Loading...")
		m.ready = true
	} else {
		m.vpA.Width = panelW
		m.vpA.Height = panelH
		m.vpB.Width = panelW
		m.vpB.Height = panelH
	}
}

func formatViewportContent(headers, body string, err error, vpWidth int) string {
	if err != nil {
		return errorStyle.Render(fmt.Sprintf("  Error: %v", err))
	}
	if headers == "" && body == "" {
		return dimStyle.Render("  (empty response)")
	}
	sep := dimStyle.Render(strings.Repeat("─", vpWidth))
	return dimStyle.Render(headers) + "\n" + sep + "\n\n" + body
}

func formatViewportContentPlain(headers, body string, err error) string {
	if err != nil {
		return "Error: " + err.Error()
	}
	if headers == "" && body == "" {
		return "(empty response)"
	}
	sep := strings.Repeat("─", 60)
	return headers + "\n" + sep + "\n\n" + body
}

func (m *responseCompareModel) setResponse(idx int, headers, body string, err error) {
	vpWidth := m.vpA.Width
	if idx == 1 {
		vpWidth = m.vpB.Width
	}
	content := formatViewportContent(headers, body, err, vpWidth)
	plain := formatViewportContentPlain(headers, body, err)
	switch idx {
	case 0:
		m.loadingA = false
		m.plainA = plain
		if m.ready {
			m.vpA.SetContent(content)
			m.vpA.GotoTop()
		}
	case 1:
		m.loadingB = false
		m.plainB = plain
		if m.ready {
			m.vpB.SetContent(content)
			m.vpB.GotoTop()
		}
	}
}

func (m responseCompareModel) plainText() string {
	var sb strings.Builder
	sb.WriteString("=== " + m.nameA + " ===\n\n")
	sb.WriteString(m.plainA)
	sb.WriteString("\n\n\n=== " + m.nameB + " ===\n\n")
	sb.WriteString(m.plainB)
	sb.WriteString("\n")
	return sb.String()
}

func (m *responseCompareModel) switchFocus() {
	m.focused = 1 - m.focused
}

func (m responseCompareModel) update(msg tea.Msg) (responseCompareModel, tea.Cmd) {
	if m.loadingA || m.loadingB {
		if _, ok := msg.(spinner.TickMsg); ok {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	if !m.ready {
		return m, nil
	}
	var cmd tea.Cmd
	if m.focused == 0 {
		m.vpA, cmd = m.vpA.Update(msg)
	} else {
		m.vpB, cmd = m.vpB.Update(msg)
	}
	return m, cmd
}

func (m responseCompareModel) view(w, h int) string {
	if !m.ready {
		return titleStyle.Render("Response Comparison") + "\n\n" + dimStyle.Render("  Initializing...")
	}

	// Header
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Response Comparison"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("One request sent to each provider with the same prompt"))
	sb.WriteString("\n\n")

	// Build each panel
	panelA := m.renderPanel(m.vpA, m.nameA, m.focused == 0, m.loadingA)
	panelB := m.renderPanel(m.vpB, m.nameB, m.focused == 1, m.loadingB)

	// Join panels side by side
	panels := lipgloss.JoinHorizontal(lipgloss.Top, panelA, "  ", panelB)
	sb.WriteString(panels)

	// Footer help
	focusedName := m.nameA
	if m.focused == 1 {
		focusedName = m.nameB
	}
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(
		fmt.Sprintf("tab switch  •  j/k scroll  •  ctrl+d/u half page  •  ctrl+e export  •  esc back    [active: %s]", focusedName),
	))

	return sb.String()
}

func (m responseCompareModel) renderPanel(vp viewport.Model, name string, focused bool, loading bool) string {
	// Panel title
	titleStr := name
	if focused {
		titleStr = selectedItemStyle.Render("▶ " + name)
	} else {
		titleStr = dimStyle.Render("  " + name)
	}

	// Scroll indicator
	scrollPct := ""
	if !loading {
		scrollPct = dimStyle.Render(fmt.Sprintf("  %3.f%%", vp.ScrollPercent()*100))
	}

	header := fmt.Sprintf("%s%s", titleStr, strings.Repeat(" ", maxInt(0, vp.Width-lipgloss.Width(titleStr)-lipgloss.Width(scrollPct))))
	header += scrollPct

	var content string
	if loading {
		content = dimStyle.Render("  " + m.spinner.View() + " Waiting for response (non-streaming)...")
	} else {
		content = vp.View()
	}

	inner := header + "\n" + content

	if focused {
		return panelFocusedBorderStyle.Width(vp.Width + 2).Render(inner)
	}
	return panelBlurredBorderStyle.Width(vp.Width + 2).Render(inner)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
