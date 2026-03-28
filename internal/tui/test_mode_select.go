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
	if m.apiMode == "completion" || m.apiMode == "anthropic_messages" {
		return 3
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
	if m.apiMode == "completion" || m.apiMode == "anthropic_messages" {
		switch m.cursor {
		case 0:
			return "single"
		case 1:
			return "single_response_view"
		case 2:
			return "pk"
		default:
			return "response_compare"
		}
	}
	switch m.cursor {
	case 0:
		return "single"
	default:
		return "pk"
	}
}

func (m testModeSelectModel) view(width, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("API Benchmark Tool"))
	sb.WriteString("\n\n")

	modeLabel := "Embedding"
	switch m.apiMode {
	case "completion":
		modeLabel = "Chat Completion"
	case "anthropic_messages":
		modeLabel = "Anthropic Messages"
	}
	sb.WriteString(subtitleStyle.Render(fmt.Sprintf("Mode: %s", modeLabel)))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Select test mode:"))
	sb.WriteString("\n\n")

	var items []struct{ label, desc string }
	if m.apiMode == "completion" || m.apiMode == "anthropic_messages" {
		items = []struct{ label, desc string }{
			{"Single Provider", "Benchmark one provider's API"},
			{"Single Response View", "Send a prompt to one provider and view the raw JSON response"},
			{"PK Mode", "Compare two providers side by side"},
			{"Response Compare", "Send a prompt to two providers and compare outputs"},
		}
	} else {
		items = []struct{ label, desc string }{
			{"Single Provider", "Benchmark one provider's API"},
			{"PK Mode", "Compare two providers side by side"},
		}
	}

	for i, item := range items {
		if i == m.cursor {
			sb.WriteString(selectedItemStyle.Render(fmt.Sprintf("  > %-24s", item.label)))
			sb.WriteString(dimStyle.Render(item.desc))
		} else {
			sb.WriteString(normalItemStyle.Render(fmt.Sprintf("    %-24s", item.label)))
			sb.WriteString(dimStyle.Render(item.desc))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("↑/↓ navigate  •  enter select  •  esc back  •  ctrl+c quit"))
	return sb.String()
}
