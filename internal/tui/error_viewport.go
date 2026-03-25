package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type errorViewportModel struct {
	vp     viewport.Model
	ready  bool
	width  int
	height int
}

func newErrorViewport() errorViewportModel {
	return errorViewportModel{}
}

func (m *errorViewportModel) setSize(w, h int) {
	m.width = w
	m.height = h
	innerW := w - 4
	innerH := h - 4
	if innerW < 10 {
		innerW = 10
	}
	if innerH < 3 {
		innerH = 3
	}
	if !m.ready {
		m.vp = viewport.New(innerW, innerH)
		m.ready = true
	} else {
		m.vp.Width = innerW
		m.vp.Height = innerH
	}
}

func (m *errorViewportModel) setContent(errorDetails map[string]int) {
	if len(errorDetails) == 0 {
		m.vp.SetContent("  No errors recorded.")
		return
	}

	type kv struct {
		msg   string
		count int
	}
	var pairs []kv
	for msg, cnt := range errorDetails {
		pairs = append(pairs, kv{msg, cnt})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].msg < pairs[j].msg
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %d distinct error type(s):\n\n", len(pairs)))
	for _, p := range pairs {
		sb.WriteString(fmt.Sprintf("  [%d×] %s\n", p.count, p.msg))
	}
	m.vp.SetContent(sb.String())
}

func (m errorViewportModel) update(msg tea.Msg) (errorViewportModel, tea.Cmd) {
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m errorViewportModel) view() string {
	if !m.ready {
		return ""
	}
	title := errorStyle.Render(" Error Log ") + dimStyle.Render(fmt.Sprintf("  scroll: ↑/↓  •  close: esc"))
	content := m.vp.View()
	scroll := fmt.Sprintf("  %3.f%%", m.vp.ScrollPercent()*100)

	inner := title + "\n" + content + "\n" + dimStyle.Render(scroll)
	return overlayStyle.
		Width(m.vp.Width + 2).
		Render(inner)
}
