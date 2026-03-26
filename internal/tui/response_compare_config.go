package tui

import (
	"encoding/json"
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
	{label: "Custom Params", placeholder: `optional JSON, e.g. {"temperature":0.7,"top_p":0.9}`},
	{label: "Provider B Name", placeholder: "e.g. Azure"},
	{label: "Provider B URL", placeholder: "https://..."},
	{label: "Provider B API Key", placeholder: "sk-...", password: true},
	{label: "Provider B Model", placeholder: "e.g. gpt-4o"},
	{label: "Custom Params", placeholder: `optional JSON, e.g. {"temperature":0.7,"top_p":0.9}`},
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
	providerA     bench.ProviderConfig
	providerB     bench.ProviderConfig
	userMessage   string
	systemPrompt  string
	customParamsA string // validated JSON object string, may be empty
	customParamsB string // validated JSON object string, may be empty
}

func validateCustomParams(raw, label string) (string, error) {
	if raw == "" {
		return "", nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", fmt.Errorf("%s Custom Params: must be a valid JSON object (%w)", label, err)
	}
	return raw, nil
}

func (m compareConfigModel) validate() (compareConfigResult, error) {
	vals := make([]string, len(m.inputs))
	for i, inp := range m.inputs {
		vals[i] = strings.TrimSpace(inp.Value())
	}

	// indices: A=0-4, B=5-9, Request=10-11
	urlA, err := bench.NormalizeURL(vals[1])
	if err != nil {
		return compareConfigResult{}, fmt.Errorf("Provider A URL: %w", err)
	}
	if vals[3] == "" {
		return compareConfigResult{}, fmt.Errorf("Provider A Model is required")
	}
	customA, err := validateCustomParams(vals[4], "Provider A")
	if err != nil {
		return compareConfigResult{}, err
	}

	urlB, err := bench.NormalizeURL(vals[6])
	if err != nil {
		return compareConfigResult{}, fmt.Errorf("Provider B URL: %w", err)
	}
	if vals[8] == "" {
		return compareConfigResult{}, fmt.Errorf("Provider B Model is required")
	}
	customB, err := validateCustomParams(vals[9], "Provider B")
	if err != nil {
		return compareConfigResult{}, err
	}

	if vals[10] == "" {
		return compareConfigResult{}, fmt.Errorf("User Message is required")
	}

	return compareConfigResult{
		providerA:     bench.ProviderConfig{Name: vals[0], URL: urlA, APIKey: vals[2], Model: vals[3], CustomParams: customA},
		providerB:     bench.ProviderConfig{Name: vals[5], URL: urlB, APIKey: vals[7], Model: vals[8], CustomParams: customB},
		userMessage:   vals[10],
		systemPrompt:  vals[11],
		customParamsA: customA,
		customParamsB: customB,
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

	renderSection("-- Provider A --", 5)
	renderSection("-- Provider B --", 5)
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
