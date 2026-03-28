package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type singleResponseModel struct {
	vp            viewport.Model
	name          string
	ready         bool
	loading       bool
	spinner       spinner.Model
	width, height int
	plain         string // uncolored text for export
}

func newSingleResponse(name string) singleResponseModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return singleResponseModel{
		name:    name,
		loading: true,
		spinner: s,
	}
}

func (m *singleResponseModel) setSize(w, h int) {
	m.width = w
	m.height = h

	vpW := w - 4
	vpH := h - 6
	if vpW < 10 {
		vpW = 10
	}
	if vpH < 3 {
		vpH = 3
	}

	if !m.ready {
		m.vp = viewport.New(vpW, vpH)
		m.vp.SetContent("  Loading...")
		m.ready = true
	} else {
		m.vp.Width = vpW
		m.vp.Height = vpH
	}
}

func (m *singleResponseModel) setResponse(headers, body string, err error) {
	m.loading = false
	m.plain = formatViewportContentPlain(headers, body, err)
	content := formatViewportContent(headers, body, err, m.vp.Width)
	if m.ready {
		m.vp.SetContent(content)
		m.vp.GotoTop()
	}
}

func (m singleResponseModel) update(msg tea.Msg) (singleResponseModel, tea.Cmd) {
	if m.loading {
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
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m singleResponseModel) view(w, h int) string {
	if !m.ready {
		return titleStyle.Render("Single Response View") + "\n\n" + dimStyle.Render("  Initializing...")
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Single Response View"))
	sb.WriteString("\n")

	providerLabel := m.name
	if providerLabel == "" {
		providerLabel = "Provider"
	}
	sb.WriteString(subtitleStyle.Render(providerLabel))
	sb.WriteString("\n\n")

	if m.loading {
		sb.WriteString(dimStyle.Render("  " + m.spinner.View() + " Waiting for response (non-streaming)..."))
		sb.WriteString("\n")
	} else {
		scrollPct := dimStyle.Render(fmt.Sprintf("  %3.f%%", m.vp.ScrollPercent()*100))
		header := strings.Repeat("─", maxInt(0, m.vp.Width-lipgloss.Width(scrollPct))) + scrollPct
		sb.WriteString(dimStyle.Render(header))
		sb.WriteString("\n")
		sb.WriteString(m.vp.View())
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("j/k scroll  •  ctrl+d/u half page  •  ctrl+e export  •  esc back  •  ctrl+c quit"))
	return sb.String()
}
