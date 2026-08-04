package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"embedding_benchmark/internal/bench"
)

type cacheHitConfigModel struct {
	apiMode    string
	inputs     []textinput.Model
	fields     []fieldDef
	prompt     textarea.Model
	focusIndex int
	err        string
	width      int
}

type cacheHitConfigResult struct {
	provider     bench.ProviderConfig
	cfg          bench.CacheHitConfig
	userPrompt   string
	customParams string
}

func buildCacheHitConfigFields(apiMode string) []fieldDef {
	urlPlaceholder := "https://api.openai.com/v1/chat/completions"
	keyPlaceholder := "sk-..."
	modelPlaceholder := "e.g. gpt-4o-mini"
	customPlaceholder := `optional JSON, e.g. {"prompt_cache_key":"benchmark"}`
	maxTokPlaceholder := "optional, e.g. 1"
	if apiMode == bench.ModeAnthropicMessages {
		urlPlaceholder = "https://api.anthropic.com/v1/messages"
		keyPlaceholder = "sk-ant-..."
		modelPlaceholder = "e.g. claude-sonnet-4-6"
		customPlaceholder = `optional JSON, e.g. {"temperature":0}`
		maxTokPlaceholder = "optional, e.g. 1  (0 → defaults to 4096)"
	}
	return []fieldDef{
		{label: "API URL", placeholder: urlPlaceholder},
		{label: "API Key", placeholder: keyPlaceholder, password: true},
		{label: "Model", placeholder: modelPlaceholder},
		{label: "Custom Params", placeholder: customPlaceholder},
		{label: "Test Count", placeholder: "e.g. 5"},
		{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
		{label: "System Prompt", placeholder: "optional, leave blank to skip"},
	}
}

func newCacheHitConfig(apiMode string) cacheHitConfigModel {
	if apiMode == "" {
		apiMode = bench.ModeCompletion
	}
	fields := buildCacheHitConfigFields(apiMode)
	inputs := make([]textinput.Model, len(fields))
	for i, fd := range fields {
		t := textinput.New()
		t.Placeholder = fd.placeholder
		t.SetValue(fd.defaultVal)
		t.CharLimit = 2048
		if fd.password {
			t.EchoMode = textinput.EchoPassword
		}
		inputs[i] = t
	}
	inputs[0].Focus()

	prompt := textarea.New()
	prompt.Placeholder = "Paste the user prompt here"
	prompt.CharLimit = 200000
	prompt.SetHeight(8)
	prompt.ShowLineNumbers = false

	return cacheHitConfigModel{apiMode: apiMode, inputs: inputs, fields: fields, prompt: prompt}
}

func (m *cacheHitConfigModel) setWidth(w int) {
	if len(m.inputs) == 0 {
		return
	}
	m.width = w
	inputW := w - 32
	if inputW < 30 {
		inputW = 30
	}
	for i := range m.inputs {
		m.inputs[i].Width = inputW
	}
	textareaW := w - 6
	if textareaW < 30 {
		textareaW = 30
	}
	m.prompt.SetWidth(textareaW)
}

func (m cacheHitConfigModel) update(msg tea.Msg) (cacheHitConfigModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.blurFocused()
			m.focusIndex = (m.focusIndex + 1) % (len(m.inputs) + 1)
			return m, m.focusFocused()
		case "shift+tab":
			m.blurFocused()
			m.focusIndex = (m.focusIndex - 1 + len(m.inputs) + 1) % (len(m.inputs) + 1)
			return m, m.focusFocused()
		}
	}
	var cmd tea.Cmd
	if m.focusIndex < len(m.inputs) {
		m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
		return m, cmd
	}
	m.prompt, cmd = m.prompt.Update(msg)
	return m, cmd
}

func (m *cacheHitConfigModel) blurFocused() {
	if m.focusIndex < len(m.inputs) {
		m.inputs[m.focusIndex].Blur()
		return
	}
	m.prompt.Blur()
}

func (m *cacheHitConfigModel) focusFocused() tea.Cmd {
	if m.focusIndex < len(m.inputs) {
		return m.inputs[m.focusIndex].Focus()
	}
	return m.prompt.Focus()
}

func (m cacheHitConfigModel) validate() (cacheHitConfigResult, error) {
	vals := make([]string, len(m.inputs))
	for i, inp := range m.inputs {
		vals[i] = strings.TrimSpace(inp.Value())
	}
	apiURL, err := bench.NormalizeURL(vals[0])
	if err != nil {
		return cacheHitConfigResult{}, fmt.Errorf("API URL: %w", err)
	}
	if vals[2] == "" {
		return cacheHitConfigResult{}, fmt.Errorf("Model is required")
	}
	customParams, err := validateCustomParams(vals[3], "Custom Params")
	if err != nil {
		return cacheHitConfigResult{}, err
	}
	testCount, err := strconv.Atoi(vals[4])
	if err != nil || testCount <= 0 {
		return cacheHitConfigResult{}, fmt.Errorf("Test Count must be a positive integer")
	}
	maxOutputTokens := 0
	if vals[5] != "" {
		maxOutputTokens, err = strconv.Atoi(vals[5])
		if err != nil || maxOutputTokens <= 0 {
			return cacheHitConfigResult{}, fmt.Errorf("Max Output Tokens must be empty or a positive integer")
		}
	}
	userPrompt := strings.TrimSpace(m.prompt.Value())
	if userPrompt == "" {
		return cacheHitConfigResult{}, fmt.Errorf("User Prompt is required")
	}

	apiMode := m.apiMode
	if apiMode == "" {
		apiMode = bench.ModeCompletion
	}

	return cacheHitConfigResult{
		provider: bench.ProviderConfig{
			Name:         "Provider",
			URL:          apiURL,
			APIKey:       vals[1],
			Model:        vals[2],
			CustomParams: customParams,
		},
		cfg: bench.CacheHitConfig{
			APIMode:         apiMode,
			TestCount:       testCount,
			MaxOutputTokens: maxOutputTokens,
			SystemPrompt:    vals[6],
		},
		userPrompt:   userPrompt,
		customParams: customParams,
	}, nil
}

func (m cacheHitConfigModel) view(width, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Prompt Cache Hit Test"))
	sb.WriteString("\n")
	subtitle := "Repeat one Chat Completions prompt every 3 seconds and read cached tokens from usage"
	if m.apiMode == bench.ModeAnthropicMessages {
		subtitle = "Repeat one Anthropic Messages prompt every 3 seconds and read cache_read / cache_creation tokens"
	}
	sb.WriteString(subtitleStyle.Render(subtitle))
	sb.WriteString("\n\n")

	renderSection := func(heading string, start, end int) {
		sb.WriteString(sectionStyle.Render(heading))
		sb.WriteString("\n")
		for i := start; i < end; i++ {
			label := labelStyle.Render(m.fields[i].label + ":")
			cursor := "  "
			if i == m.focusIndex {
				cursor = "> "
			}
			sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, m.inputs[i].View()))
		}
	}
	renderSection("-- Provider --", 0, 4)
	renderSection("-- Request --", 4, len(m.inputs))

	cursor := "  "
	if m.focusIndex == len(m.inputs) {
		cursor = "> "
	}
	sb.WriteString(sectionStyle.Render("-- User Prompt --"))
	sb.WriteString("\n")
	sb.WriteString(cursor)
	sb.WriteString(m.prompt.View())
	sb.WriteString("\n")

	if m.err != "" {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render("Error: " + m.err))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("tab/shift+tab navigate  •  ctrl+s start test  •  esc back  •  ctrl+c quit"))
	return sb.String()
}
