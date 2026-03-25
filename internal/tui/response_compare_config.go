package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"embedding_benchmark/internal/bench"
)

var compareConfigFields = []fieldDef{
	{label: "Provider A Name", placeholder: "e.g. OpenAI"},
	{label: "Provider A URL", placeholder: "https://api.openai.com/v1/chat/completions"},
	{label: "Provider A API Key", placeholder: "sk-...", password: true},
	{label: "Provider A Model", placeholder: "e.g. gpt-4o-mini"},
	{label: "Provider B Name", placeholder: "e.g. Azure"},
	{label: "Provider B URL", placeholder: "https://..."},
	{label: "Provider B API Key", placeholder: "sk-...", password: true},
	{label: "Provider B Model", placeholder: "e.g. gpt-4o"},
	{label: "User Message", placeholder: "Enter the prompt to send to both providers"},
	{label: "System Prompt", placeholder: "optional, leave blank to skip"},
}

type compareConfigModel struct {
	inputs     []textinput.Model
	focusIndex int
	err        string
	width      int
}

func newCompareConfig() compareConfigModel {
	inputs := make([]textinput.Model, len(compareConfigFields))
	for i, fd := range compareConfigFields {
		t := textinput.New()
		t.Placeholder = fd.placeholder
		t.CharLimit = 2048
		if fd.password {
			t.EchoMode = textinput.EchoPassword
		}
		inputs[i] = t
	}
	inputs[0].Focus()
	return compareConfigModel{inputs: inputs}
}

func (m *compareConfigModel) setWidth(w int) { m.width = w }

func (m compareConfigModel) update(msg tea.Msg) (compareConfigModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.inputs[m.focusIndex].Blur()
			m.focusIndex = (m.focusIndex + 1) % len(m.inputs)
			return m, m.inputs[m.focusIndex].Focus()
		case "shift+tab":
			m.inputs[m.focusIndex].Blur()
			m.focusIndex = (m.focusIndex - 1 + len(m.inputs)) % len(m.inputs)
			return m, m.inputs[m.focusIndex].Focus()
		}
	}
	var cmd tea.Cmd
	m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
	return m, cmd
}

type compareConfigResult struct {
	providerA    bench.ProviderConfig
	providerB    bench.ProviderConfig
	userMessage  string
	systemPrompt string
}

func (m compareConfigModel) validate() (compareConfigResult, error) {
	vals := make([]string, len(m.inputs))
	for i, inp := range m.inputs {
		vals[i] = strings.TrimSpace(inp.Value())
	}

	urlA, err := bench.NormalizeURL(vals[1])
	if err != nil {
		return compareConfigResult{}, fmt.Errorf("Provider A URL: %w", err)
	}
	if vals[3] == "" {
		return compareConfigResult{}, fmt.Errorf("Provider A Model is required")
	}
	urlB, err := bench.NormalizeURL(vals[5])
	if err != nil {
		return compareConfigResult{}, fmt.Errorf("Provider B URL: %w", err)
	}
	if vals[7] == "" {
		return compareConfigResult{}, fmt.Errorf("Provider B Model is required")
	}
	if vals[8] == "" {
		return compareConfigResult{}, fmt.Errorf("User Message is required")
	}

	return compareConfigResult{
		providerA:    bench.ProviderConfig{Name: vals[0], URL: urlA, APIKey: vals[2], Model: vals[3]},
		providerB:    bench.ProviderConfig{Name: vals[4], URL: urlB, APIKey: vals[6], Model: vals[7]},
		userMessage:  vals[8],
		systemPrompt: vals[9],
	}, nil
}

func (m compareConfigModel) view(width, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Response Compare"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Send the same prompt to two providers and compare their outputs"))
	sb.WriteString("\n\n")

	i := 0
	renderSection := func(heading string, count int) {
		if heading != "" {
			sb.WriteString(sectionStyle.Render(heading))
			sb.WriteString("\n")
		}
		for end := i + count; i < end && i < len(m.inputs); i++ {
			label := labelStyle.Render(compareConfigFields[i].label + ":")
			inp := m.inputs[i].View()
			cursor := "  "
			if i == m.focusIndex {
				cursor = "> "
			}
			sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, inp))
		}
	}

	renderSection("-- Provider A --", 4)
	renderSection("-- Provider B --", 4)
	renderSection("-- Request --", 2)

	if m.err != "" {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render("Error: " + m.err))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("tab/shift+tab navigate  •  ctrl+s send  •  esc back  •  ctrl+c quit"))
	return sb.String()
}
