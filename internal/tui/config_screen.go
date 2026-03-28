package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"embedding_benchmark/internal/bench"
)

type fieldDef struct {
	label       string
	placeholder string
	defaultVal  string
	password    bool
}

type configModel struct {
	apiMode    string
	testMode   string
	inputs     []textinput.Model
	fieldDefs  []fieldDef
	focusIndex int
	err        string
	width      int
}

func newConfigModel(apiMode, testMode string) configModel {
	m := configModel{
		apiMode:  apiMode,
		testMode: testMode,
	}
	m.fieldDefs = buildFieldDefs(apiMode, testMode)
	m.inputs = make([]textinput.Model, len(m.fieldDefs))
	for i, fd := range m.fieldDefs {
		t := textinput.New()
		t.Placeholder = fd.placeholder
		t.SetValue(fd.defaultVal)
		t.CharLimit = 512
		if fd.password {
			t.EchoMode = textinput.EchoPassword
		}
		m.inputs[i] = t
	}
	if len(m.inputs) > 0 {
		m.inputs[0].Focus()
	}
	return m
}

func buildFieldDefs(apiMode, testMode string) []fieldDef {
	isCompletion    := apiMode == "completion"
	isAnthropicMsg  := apiMode == "anthropic_messages"
	isCompletionLike := isCompletion || isAnthropicMsg
	isPK := testMode == "pk"

	customParamsPlaceholder := `optional JSON, e.g. {"temperature":0.7}`

	maxTokPlaceholder := "e.g. 256  (0 = unlimited)"
	if isAnthropicMsg {
		maxTokPlaceholder = "e.g. 4096  (0 → defaults to 4096)"
	}

	if isPK {
		urlPlaceholderA := "https://api.openai.com/v1/embeddings"
		modelPlaceholderA := "e.g. text-embedding-3-small"
		urlPlaceholderB := "https://..."
		modelPlaceholderB := "e.g. text-embedding-ada-002"
		keyPlaceholder := "sk-..."
		if isCompletion {
			urlPlaceholderA = "https://api.openai.com/v1/chat/completions"
			modelPlaceholderA = "e.g. gpt-4o-mini"
			urlPlaceholderB = "https://..."
			modelPlaceholderB = "e.g. gpt-4o-mini"
		} else if isAnthropicMsg {
			urlPlaceholderA = "https://api.anthropic.com/v1/messages"
			modelPlaceholderA = "e.g. claude-sonnet-4-6"
			urlPlaceholderB = "https://api.anthropic.com/v1/messages"
			modelPlaceholderB = "e.g. claude-haiku-4-5"
			keyPlaceholder = "sk-ant-..."
		}
		defs := []fieldDef{
			{label: "Provider A Name", placeholder: "e.g. Provider A"},
			{label: "Provider A URL", placeholder: urlPlaceholderA},
			{label: "Provider A API Key", placeholder: keyPlaceholder, password: true},
			{label: "Provider A Model", placeholder: modelPlaceholderA},
			{label: "Custom Params", placeholder: customParamsPlaceholder},
			{label: "Provider B Name", placeholder: "e.g. Provider B"},
			{label: "Provider B URL", placeholder: urlPlaceholderB},
			{label: "Provider B API Key", placeholder: keyPlaceholder, password: true},
			{label: "Provider B Model", placeholder: modelPlaceholderB},
			{label: "Custom Params", placeholder: customParamsPlaceholder},
			{label: "Concurrency", placeholder: "e.g. 10"},
			{label: "Total Requests", placeholder: "e.g. 100"},
			{label: "Input Tokens", placeholder: "e.g. 500"},
		}
		if isCompletionLike {
			defs = append(defs,
				fieldDef{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
				fieldDef{label: "System Prompt", placeholder: "optional, leave blank to skip"},
			)
		}
		return defs
	}

	// Single provider
	urlPlaceholder := "https://api.openai.com/v1/embeddings"
	modelPlaceholder := "e.g. text-embedding-3-small"
	keyPlaceholder := "sk-..."
	if isCompletion {
		urlPlaceholder = "https://api.openai.com/v1/chat/completions"
		modelPlaceholder = "e.g. gpt-4o-mini"
	} else if isAnthropicMsg {
		urlPlaceholder = "https://api.anthropic.com/v1/messages"
		modelPlaceholder = "e.g. claude-sonnet-4-6"
		keyPlaceholder = "sk-ant-..."
	}
	defs := []fieldDef{
		{label: "API URL", placeholder: urlPlaceholder},
		{label: "API Key", placeholder: keyPlaceholder, password: true},
		{label: "Model", placeholder: modelPlaceholder},
		{label: "Custom Params", placeholder: customParamsPlaceholder},
		{label: "Concurrency", placeholder: "e.g. 10"},
		{label: "Total Requests", placeholder: "e.g. 100"},
		{label: "Input Tokens", placeholder: "e.g. 500"},
	}
	if isCompletionLike {
		defs = append(defs,
			fieldDef{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
			fieldDef{label: "System Prompt", placeholder: "optional, leave blank to skip"},
		)
	}
	return defs
}

func (m *configModel) setWidth(w int) {
	m.width = w
}

func (m configModel) update(msg tea.Msg) (configModel, tea.Cmd) {
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

	// Delegate to focused input
	var cmd tea.Cmd
	m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
	return m, cmd
}

// validate returns (providers, cfg, err) if valid.
func (m configModel) validate() ([]bench.ProviderConfig, bench.BenchConfig, error) {
	vals := make([]string, len(m.inputs))
	for i, inp := range m.inputs {
		vals[i] = strings.TrimSpace(inp.Value())
	}

	isPK            := m.testMode == "pk"
	isCompletion    := m.apiMode == "completion"
	isAnthropicMsg  := m.apiMode == "anthropic_messages"
	isCompletionLike := isCompletion || isAnthropicMsg

	parseInt := func(s, name string) (int, error) {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", name)
		}
		return v, nil
	}

	if isPK {
		// indices: A=0-4, B=5-9, shared=10+
		urlA, err := bench.NormalizeURL(vals[1])
		if err != nil {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider A URL: %w", err)
		}
		if vals[3] == "" {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider A Model is required")
		}
		if _, err := validateCustomParams(vals[4], "Provider A"); err != nil {
			return nil, bench.BenchConfig{}, err
		}
		urlB, err := bench.NormalizeURL(vals[6])
		if err != nil {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider B URL: %w", err)
		}
		if vals[8] == "" {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider B Model is required")
		}
		if _, err := validateCustomParams(vals[9], "Provider B"); err != nil {
			return nil, bench.BenchConfig{}, err
		}
		c, err := parseInt(vals[10], "Concurrency")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
		n, err := parseInt(vals[11], "Total Requests")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
		tokens, err := parseInt(vals[12], "Input Tokens")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}

		mode := bench.ModeEmbedding
		if isCompletion   { mode = bench.ModeCompletion }
		if isAnthropicMsg { mode = bench.ModeAnthropicMessages }
		cfg := bench.BenchConfig{
			Mode:          mode,
			Concurrency:   c,
			TotalRequests: n,
			TargetTokens:  tokens,
		}
		if isCompletionLike {
			maxTok, _ := strconv.Atoi(vals[13])
			cfg.MaxOutputTokens = maxTok
			cfg.SystemPrompt = vals[14]
		}
		providers := []bench.ProviderConfig{
			{Name: vals[0], URL: urlA, APIKey: vals[2], Model: vals[3], CustomParams: vals[4]},
			{Name: vals[5], URL: urlB, APIKey: vals[7], Model: vals[8], CustomParams: vals[9]},
		}
		return providers, cfg, nil
	}

	// Single provider — indices: 0=URL, 1=Key, 2=Model, 3=CustomParams, 4=Concurrency, ...
	apiURL, err := bench.NormalizeURL(vals[0])
	if err != nil {
		return nil, bench.BenchConfig{}, fmt.Errorf("API URL: %w", err)
	}
	if vals[2] == "" {
		return nil, bench.BenchConfig{}, fmt.Errorf("Model is required")
	}
	if _, err := validateCustomParams(vals[3], "Custom Params"); err != nil {
		return nil, bench.BenchConfig{}, err
	}
	c, err := parseInt(vals[4], "Concurrency")
	if err != nil {
		return nil, bench.BenchConfig{}, err
	}
	n, err := parseInt(vals[5], "Total Requests")
	if err != nil {
		return nil, bench.BenchConfig{}, err
	}
	tokens, err := parseInt(vals[6], "Input Tokens")
	if err != nil {
		return nil, bench.BenchConfig{}, err
	}

	mode := bench.ModeEmbedding
	if isCompletion   { mode = bench.ModeCompletion }
	if isAnthropicMsg { mode = bench.ModeAnthropicMessages }
	cfg := bench.BenchConfig{
		Mode:          mode,
		Concurrency:   c,
		TotalRequests: n,
		TargetTokens:  tokens,
	}
	if isCompletionLike {
		maxTok, _ := strconv.Atoi(vals[7])
		cfg.MaxOutputTokens = maxTok
		cfg.SystemPrompt = vals[8]
	}
	providers := []bench.ProviderConfig{
		{Name: "Provider", URL: apiURL, APIKey: vals[1], Model: vals[2], CustomParams: vals[3]},
	}
	return providers, cfg, nil
}

func (m configModel) view(width, height int) string {
	var sb strings.Builder

	apiLabel := "Embedding"
	switch m.apiMode {
	case "completion":
		apiLabel = "Chat Completion"
	case "anthropic_messages":
		apiLabel = "Anthropic Messages"
	}
	testLabel := "Single Provider"
	if m.testMode == "pk" {
		testLabel = "PK Mode"
	}
	sb.WriteString(titleStyle.Render("Configure Benchmark"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render(fmt.Sprintf("%s  •  %s", apiLabel, testLabel)))
	sb.WriteString("\n\n")

	isPK := m.testMode == "pk"
	i := 0
	renderSection := func(heading string, count int) {
		if heading != "" {
			sb.WriteString(sectionStyle.Render(heading))
			sb.WriteString("\n")
		}
		for end := i + count; i < end && i < len(m.inputs); i++ {
			label := labelStyle.Render(m.fieldDefs[i].label + ":")
			inp := m.inputs[i].View()
			focused := i == m.focusIndex
			cursor := "  "
			if focused {
				cursor = "> "
			}
			sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, inp))
		}
	}

	if isPK {
		renderSection("-- Provider A --", 5)
		renderSection("-- Provider B --", 5)
		sharedCount := len(m.fieldDefs) - 10
		renderSection("-- Shared Parameters --", sharedCount)
	} else {
		renderSection("", len(m.fieldDefs))
	}

	if m.err != "" {
		sb.WriteString("\n")
		sb.WriteString(errorStyle.Render("Error: " + m.err))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("tab/shift+tab navigate  •  ctrl+s start benchmark  •  esc back  •  ctrl+c quit"))
	return sb.String()
}
