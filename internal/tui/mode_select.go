package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type modeSelectModel struct {
	cursor int
	items  []string
}

func newModeSelect() modeSelectModel {
	return modeSelectModel{
		items: []string{"Embedding", "Chat Completion", "Anthropic Messages"},
	}
}

func (m modeSelectModel) update(msg tea.Msg) (modeSelectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m modeSelectModel) selected() string {
	switch m.cursor {
	case 0:
		return "embedding"
	case 1:
		return "completion"
	case 2:
		return "anthropic_messages"
	}
	return "embedding"
}

func (m modeSelectModel) view(width, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("API Benchmark Tool"))
	sb.WriteString("\n\n")
	sb.WriteString(subtitleStyle.Render("Select API type to benchmark:"))
	sb.WriteString("\n\n")

	for i, item := range m.items {
		if i == m.cursor {
			sb.WriteString(selectedItemStyle.Render(fmt.Sprintf("  > %s", item)))
		} else {
			sb.WriteString(normalItemStyle.Render(fmt.Sprintf("    %s", item)))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("↑/↓ navigate  •  enter select  •  ctrl+c quit"))
	return sb.String()
}
