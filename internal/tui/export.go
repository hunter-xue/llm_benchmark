package tui

import (
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

type exportModel struct {
	input   textinput.Model
	active  bool
	content string // plain text to write
	status  string // feedback shown after write
	isErr   bool
}

func newExportModel() exportModel {
	t := textinput.New()
	t.Placeholder = "e.g. result.txt"
	t.CharLimit = 256
	return exportModel{input: t}
}

func (m *exportModel) activate(content string) tea.Cmd {
	m.active = true
	m.content = content
	m.status = ""
	m.isErr = false
	m.input.SetValue("")
	return m.input.Focus()
}

func (m *exportModel) deactivate() {
	m.active = false
	m.status = ""
	m.input.Blur()
}

func (m exportModel) update(msg tea.Msg) (exportModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			filename := strings.TrimSpace(m.input.Value())
			if filename == "" {
				m.status = "filename cannot be empty"
				m.isErr = true
				return m, nil
			}
			if err := os.WriteFile(filename, []byte(m.content), 0644); err != nil {
				m.status = "error: " + err.Error()
				m.isErr = true
			} else {
				m.status = "saved → " + filename
				m.isErr = false
				m.active = false
				m.input.Blur()
			}
			return m, nil
		case "esc":
			m.deactivate()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// view renders the export prompt or status bar (replaces the normal help line).
func (m exportModel) view() string {
	if m.active {
		prompt := labelStyle.Render("Export to:") + "  " + m.input.View()
		hint := helpStyle.Render("enter save  •  esc cancel")
		return prompt + "\n" + hint
	}
	if m.status != "" {
		if m.isErr {
			return errorStyle.Render("  " + m.status)
		}
		return successStyle.Render("  ✓ " + m.status)
	}
	return ""
}
