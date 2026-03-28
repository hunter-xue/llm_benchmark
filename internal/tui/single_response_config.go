package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"embedding_benchmark/internal/bench"
)

func buildSingleResponseConfigFields(apiMode string) []fieldDef {
	customParamsPlaceholder := `optional JSON, e.g. {"temperature":0.7}`
	if apiMode == "anthropic_messages" {
		return []fieldDef{
			{label: "API URL", placeholder: "https://api.anthropic.com/v1/messages"},
			{label: "API Key", placeholder: "sk-ant-...", password: true},
			{label: "Model", placeholder: "e.g. claude-sonnet-4-6"},
			{label: "Custom Params", placeholder: customParamsPlaceholder},
			{label: "User Message", placeholder: "Enter the prompt to send"},
			{label: "System Prompt", placeholder: "optional, leave blank to skip"},
		}
	}
	return []fieldDef{
		{label: "API URL", placeholder: "https://api.openai.com/v1/chat/completions"},
		{label: "API Key", placeholder: "sk-...", password: true},
		{label: "Model", placeholder: "e.g. gpt-4o-mini"},
		{label: "Custom Params", placeholder: customParamsPlaceholder},
		{label: "User Message", placeholder: "Enter the prompt to send"},
		{label: "System Prompt", placeholder: "optional, leave blank to skip"},
	}
}

type singleResponseConfigModel struct {
	inputs     []textinput.Model
	fields     []fieldDef
	focusIndex int
	err        string
	width      int
}

func newSingleResponseConfig(apiMode string) singleResponseConfigModel {
	fields := buildSingleResponseConfigFields(apiMode)
	inputs := make([]textinput.Model, len(fields))
	for i, fd := range fields {
		t := textinput.New()
		t.Placeholder = fd.placeholder
		t.CharLimit = 2048
		if fd.password {
			t.EchoMode = textinput.EchoPassword
		}
		inputs[i] = t
	}
	inputs[0].Focus()
	return singleResponseConfigModel{inputs: inputs, fields: fields}
}

func (m *singleResponseConfigModel) setWidth(w int) { m.width = w }

func (m singleResponseConfigModel) update(msg tea.Msg) (singleResponseConfigModel, tea.Cmd) {
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

type singleResponseConfigResult struct {
	provider     bench.ProviderConfig
	userMessage  string
	systemPrompt string
}

func (m singleResponseConfigModel) validate() (singleResponseConfigResult, error) {
	vals := make([]string, len(m.inputs))
	for i, inp := range m.inputs {
		vals[i] = strings.TrimSpace(inp.Value())
	}

	// indices: 0=URL, 1=Key, 2=Model, 3=CustomParams, 4=UserMessage, 5=SystemPrompt
	apiURL, err := bench.NormalizeURL(vals[0])
	if err != nil {
		return singleResponseConfigResult{}, fmt.Errorf("API URL: %w", err)
	}
	if vals[2] == "" {
		return singleResponseConfigResult{}, fmt.Errorf("Model is required")
	}
	customParams, err := validateCustomParams(vals[3], "Custom Params")
	if err != nil {
		return singleResponseConfigResult{}, err
	}
	if vals[4] == "" {
		return singleResponseConfigResult{}, fmt.Errorf("User Message is required")
	}

	return singleResponseConfigResult{
		provider:     bench.ProviderConfig{URL: apiURL, APIKey: vals[1], Model: vals[2], CustomParams: customParams},
		userMessage:  vals[4],
		systemPrompt: vals[5],
	}, nil
}

func (m singleResponseConfigModel) view(width, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Single Response View"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Send a prompt to one provider and view the raw JSON response"))
	sb.WriteString("\n\n")

	i := 0
	renderSection := func(heading string, count int) {
		if heading != "" {
			sb.WriteString(sectionStyle.Render(heading))
			sb.WriteString("\n")
		}
		for end := i + count; i < end && i < len(m.inputs); i++ {
			label := labelStyle.Render(m.fields[i].label + ":")
			inp := m.inputs[i].View()
			cursor := "  "
			if i == m.focusIndex {
				cursor = "> "
			}
			sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, inp))
		}
	}

	renderSection("-- Provider --", 4)
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
