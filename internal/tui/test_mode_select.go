package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type testModeSelectModel struct {
	cursor  int
	apiMode string
}

func newTestModeSelect(apiMode string) testModeSelectModel {
	return testModeSelectModel{apiMode: apiMode}
}

func (m testModeSelectModel) maxCursor() int {
	if m.apiMode == "completion" {
		return 2
	}
	return 1
}

func (m testModeSelectModel) update(msg tea.Msg) (testModeSelectModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.maxCursor() {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m testModeSelectModel) selected() string {
	switch m.cursor {
	case 0:
		return "single"
	case 1:
		return "pk"
	default:
		return "response_compare"
	}
}

func (m testModeSelectModel) view(width, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("API Benchmark Tool"))
	sb.WriteString("\n\n")

	modeLabel := "Embedding"
	if m.apiMode == "completion" {
		modeLabel = "Chat Completion"
	}
	sb.WriteString(subtitleStyle.Render(fmt.Sprintf("Mode: %s", modeLabel)))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Select test mode:"))
	sb.WriteString("\n\n")

	items := []struct{ label, desc string }{
		{"Single Provider", "Benchmark one provider's API"},
		{"PK Mode", "Compare two providers side by side"},
	}
	if m.apiMode == "completion" {
		items = append(items, struct{ label, desc string }{
			"Response Compare", "Send a prompt to two providers and compare outputs",
		})
	}

	for i, item := range items {
		if i == m.cursor {
			sb.WriteString(selectedItemStyle.Render(fmt.Sprintf("  > %-20s", item.label)))
			sb.WriteString(dimStyle.Render(item.desc))
		} else {
			sb.WriteString(normalItemStyle.Render(fmt.Sprintf("    %-20s", item.label)))
			sb.WriteString(dimStyle.Render(item.desc))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("↑/↓ navigate  •  enter select  •  esc back  •  ctrl+c quit"))
	return sb.String()
}
