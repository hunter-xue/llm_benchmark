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
	fieldType   string // "" (textinput, default) or "toggle"
}

type configModel struct {
	apiMode         string
	testMode        string
	inputs          []textinput.Model
	fieldDefs       []fieldDef
	focusIndex      int
	err             string
	width           int
	loadModelIndex  int  // 0 = closed-loop, 1 = open-loop (completion only)
	ttftReasoningOn bool // TTFT Includes Reasoning toggle (completion-like modes)
}

func newConfigModel(apiMode, testMode string) configModel {
	m := configModel{
		apiMode:         apiMode,
		testMode:        testMode,
		ttftReasoningOn: true,
	}
	m.rebuildFields()
	if len(m.inputs) > 0 {
		m.inputs[0].Focus()
	}
	return m
}

// rebuildFields constructs fieldDefs and inputs based on the current
// apiMode/testMode/loadModelIndex. Existing input values are preserved by
// label (in order) so toggling the load model does not wipe user input.
func (m *configModel) rebuildFields() {
	oldValues := make(map[string][]string)
	for i, fd := range m.fieldDefs {
		oldValues[fd.label] = append(oldValues[fd.label], m.inputs[i].Value())
	}

	m.fieldDefs = buildFieldDefs(m.apiMode, m.testMode, m.loadModelIndex)
	m.inputs = make([]textinput.Model, len(m.fieldDefs))
	for i, fd := range m.fieldDefs {
		t := textinput.New()
		t.Placeholder = fd.placeholder
		t.CharLimit = 512
		if fd.password {
			t.EchoMode = textinput.EchoPassword
		}
		val := fd.defaultVal
		if vals := oldValues[fd.label]; len(vals) > 0 {
			val = vals[0]
			oldValues[fd.label] = vals[1:]
		}
		t.SetValue(val)
		m.inputs[i] = t
	}
	// Clamp focusIndex
	if m.focusIndex >= len(m.inputs) {
		m.focusIndex = len(m.inputs) - 1
	}
	if m.focusIndex < 0 {
		m.focusIndex = 0
	}
}

func buildFieldDefs(apiMode, testMode string, loadModelIndex int) []fieldDef {
	isCompletion := apiMode == "completion"
	isAnthropicMsg := apiMode == "anthropic_messages"
	isCompletionLike := isCompletion || isAnthropicMsg
	isPK := testMode == "pk"
	isOpenLoop := isCompletion && loadModelIndex == 1

	customParamsPlaceholder := `optional JSON, e.g. {"temperature":0.7}`

	maxTokPlaceholder := "e.g. 256  (0 = unlimited)"
	if isAnthropicMsg {
		maxTokPlaceholder = "e.g. 4096  (0 → defaults to 4096)"
	}

	// Load Model toggle field (only for completion mode)
	loadModelField := fieldDef{
		label:     "Load Model",
		fieldType: "toggle",
	}

	// TTFT reasoning toggle field (completion-like modes)
	ttftReasoningField := fieldDef{
		label:     "TTFT Includes Reasoning",
		fieldType: "toggle",
	}

	// Concurrency fields for closed-loop vs open-loop
	concurrencyField := fieldDef{label: "Concurrency", placeholder: "e.g. 10"}
	maxInFlightField := fieldDef{label: "Max In-Flight", placeholder: "equivalent to Concurrency, e.g. 10"}
	requestRateField := fieldDef{label: "Request Rate", placeholder: "requests per second, e.g. 10"}

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
		}
		// Add Load Model toggle for completion mode
		if isCompletion {
			defs = append(defs, loadModelField)
		}
		// Add concurrency fields based on load model
		if isOpenLoop {
			defs = append(defs, maxInFlightField, requestRateField)
		} else {
			defs = append(defs, concurrencyField)
		}
		defs = append(defs,
			fieldDef{label: "Total Requests", placeholder: "e.g. 100"},
			fieldDef{label: "Input Tokens", placeholder: "e.g. 500"},
		)
		if isCompletionLike {
			defs = append(defs,
				fieldDef{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
				ttftReasoningField,
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
	}
	// Add Load Model toggle for completion mode
	if isCompletion {
		defs = append(defs, loadModelField)
	}
	// Add concurrency fields based on load model
	if isOpenLoop {
		defs = append(defs, maxInFlightField, requestRateField)
	} else {
		defs = append(defs, concurrencyField)
	}
	defs = append(defs,
		fieldDef{label: "Total Requests", placeholder: "e.g. 100"},
		fieldDef{label: "Input Tokens", placeholder: "e.g. 500"},
	)
	if isCompletionLike {
		defs = append(defs,
			fieldDef{label: "Max Output Tokens", placeholder: maxTokPlaceholder},
			ttftReasoningField,
			fieldDef{label: "System Prompt", placeholder: "optional, leave blank to skip"},
		)
	}
	return defs
}

func (m *configModel) setWidth(w int) {
	m.width = w
}

// isToggleField returns true if the currently focused field is a toggle.
func (m configModel) isToggleField() bool {
	if m.focusIndex < 0 || m.focusIndex >= len(m.fieldDefs) {
		return false
	}
	return m.fieldDefs[m.focusIndex].fieldType == "toggle"
}

// toggleLoadModel switches between closed-loop and open-loop, rebuilding fields.
func (m *configModel) toggleLoadModel() {
	if m.apiMode != bench.ModeCompletion {
		return // only completion mode supports open-loop
	}
	m.loadModelIndex = 1 - m.loadModelIndex // toggle 0↔1
	m.rebuildFields()
}

// toggleFocusedField flips the state of the currently focused toggle field.
func (m *configModel) toggleFocusedField() {
	if m.focusIndex < 0 || m.focusIndex >= len(m.fieldDefs) {
		return
	}
	switch m.fieldDefs[m.focusIndex].label {
	case "Load Model":
		m.toggleLoadModel()
	case "TTFT Includes Reasoning":
		m.ttftReasoningOn = !m.ttftReasoningOn
	default:
		panic(fmt.Sprintf("no toggle handler for field %q", m.fieldDefs[m.focusIndex].label))
	}
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
		case "left", "right":
			if m.isToggleField() {
				m.toggleFocusedField()
				return m, nil
			}
		}
	}

	// Delegate to focused input (toggle fields have no textinput to update)
	if !m.isToggleField() {
		var cmd tea.Cmd
		m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
		return m, cmd
	}
	return m, nil
}

// validate returns (providers, cfg, err) if valid.
func (m configModel) validate() ([]bench.ProviderConfig, bench.BenchConfig, error) {
	vals := make([]string, len(m.inputs))
	for i, inp := range m.inputs {
		vals[i] = strings.TrimSpace(inp.Value())
	}

	isPK := m.testMode == "pk"
	isCompletion := m.apiMode == "completion"
	isAnthropicMsg := m.apiMode == "anthropic_messages"
	isCompletionLike := isCompletion || isAnthropicMsg
	isOpenLoop := isCompletion && m.loadModelIndex == 1

	parseInt := func(s, name string) (int, error) {
		v, err := strconv.Atoi(s)
		if err != nil || v <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", name)
		}
		return v, nil
	}

	// fieldIdx finds a field index by label since positions shift based on
	// the load model. Returns the first match. Panics on missing label to
	// surface programming errors immediately rather than crashing with an
	// index-out-of-range at an unrelated line.
	fieldIdx := func(label string) int {
		for i, fd := range m.fieldDefs {
			if fd.label == label {
				return i
			}
		}
		panic(fmt.Sprintf("field %q not found in fieldDefs", label))
	}

	if isPK {
		urlAIdx := fieldIdx("Provider A URL")
		modelAIdx := fieldIdx("Provider A Model")
		customAIdx := fieldIdx("Custom Params") // first Custom Params (Provider A)
		urlBIdx := fieldIdx("Provider B URL")
		modelBIdx := fieldIdx("Provider B Model")
		// Second Custom Params belongs to Provider B
		customBIdx := -1
		for i, fd := range m.fieldDefs {
			if fd.label == "Custom Params" && i > customAIdx {
				customBIdx = i
				break
			}
		}

		urlA, err := bench.NormalizeURL(vals[urlAIdx])
		if err != nil {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider A URL: %w", err)
		}
		if vals[modelAIdx] == "" {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider A Model is required")
		}
		if _, err := validateCustomParams(vals[customAIdx], "Provider A"); err != nil {
			return nil, bench.BenchConfig{}, err
		}
		urlB, err := bench.NormalizeURL(vals[urlBIdx])
		if err != nil {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider B URL: %w", err)
		}
		if vals[modelBIdx] == "" {
			return nil, bench.BenchConfig{}, fmt.Errorf("Provider B Model is required")
		}
		if _, err := validateCustomParams(vals[customBIdx], "Provider B"); err != nil {
			return nil, bench.BenchConfig{}, err
		}

		var c, rate int
		if isOpenLoop {
			c, err = parseInt(vals[fieldIdx("Max In-Flight")], "Max In-Flight")
			if err != nil {
				return nil, bench.BenchConfig{}, err
			}
			rate, err = parseInt(vals[fieldIdx("Request Rate")], "Request Rate")
			if err != nil {
				return nil, bench.BenchConfig{}, err
			}
		} else {
			c, err = parseInt(vals[fieldIdx("Concurrency")], "Concurrency")
			if err != nil {
				return nil, bench.BenchConfig{}, err
			}
		}

		n, err := parseInt(vals[fieldIdx("Total Requests")], "Total Requests")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
		tokens, err := parseInt(vals[fieldIdx("Input Tokens")], "Input Tokens")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}

		mode := bench.ModeEmbedding
		if isCompletion {
			mode = bench.ModeCompletion
		}
		if isAnthropicMsg {
			mode = bench.ModeAnthropicMessages
		}
		cfg := bench.BenchConfig{
			Mode:          mode,
			Concurrency:   c,
			TotalRequests: n,
			TargetTokens:  tokens,
		}
		if isOpenLoop {
			cfg.LoadModel = bench.LoadModelOpenLoop
			cfg.MaxInFlight = c
			cfg.RequestRate = rate
		} else {
			cfg.LoadModel = bench.LoadModelClosedLoop
		}
		if isCompletionLike {
			maxTok, _ := strconv.Atoi(vals[fieldIdx("Max Output Tokens")])
			cfg.MaxOutputTokens = maxTok
			cfg.SystemPrompt = vals[fieldIdx("System Prompt")]
			cfg.TTFTIncludesReasoning = m.ttftReasoningOn
		}
		providers := []bench.ProviderConfig{
			{Name: vals[fieldIdx("Provider A Name")], URL: urlA, APIKey: vals[fieldIdx("Provider A API Key")], Model: vals[modelAIdx], CustomParams: vals[customAIdx]},
			{Name: vals[fieldIdx("Provider B Name")], URL: urlB, APIKey: vals[fieldIdx("Provider B API Key")], Model: vals[modelBIdx], CustomParams: vals[customBIdx]},
		}
		return providers, cfg, nil
	}

	// Single provider — find indices by label
	urlIdx := fieldIdx("API URL")
	keyIdx := fieldIdx("API Key")
	modelIdx := fieldIdx("Model")
	customIdx := fieldIdx("Custom Params")

	apiURL, err := bench.NormalizeURL(vals[urlIdx])
	if err != nil {
		return nil, bench.BenchConfig{}, fmt.Errorf("API URL: %w", err)
	}
	if vals[modelIdx] == "" {
		return nil, bench.BenchConfig{}, fmt.Errorf("Model is required")
	}
	if _, err := validateCustomParams(vals[customIdx], "Custom Params"); err != nil {
		return nil, bench.BenchConfig{}, err
	}

	var c, rate int
	if isOpenLoop {
		c, err = parseInt(vals[fieldIdx("Max In-Flight")], "Max In-Flight")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
		rate, err = parseInt(vals[fieldIdx("Request Rate")], "Request Rate")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
	} else {
		c, err = parseInt(vals[fieldIdx("Concurrency")], "Concurrency")
		if err != nil {
			return nil, bench.BenchConfig{}, err
		}
	}

	n, err := parseInt(vals[fieldIdx("Total Requests")], "Total Requests")
	if err != nil {
		return nil, bench.BenchConfig{}, err
	}
	tokens, err := parseInt(vals[fieldIdx("Input Tokens")], "Input Tokens")
	if err != nil {
		return nil, bench.BenchConfig{}, err
	}

	mode := bench.ModeEmbedding
	if isCompletion {
		mode = bench.ModeCompletion
	}
	if isAnthropicMsg {
		mode = bench.ModeAnthropicMessages
	}
	cfg := bench.BenchConfig{
		Mode:          mode,
		Concurrency:   c,
		TotalRequests: n,
		TargetTokens:  tokens,
	}
	if isOpenLoop {
		cfg.LoadModel = bench.LoadModelOpenLoop
		cfg.MaxInFlight = c
		cfg.RequestRate = rate
	} else {
		cfg.LoadModel = bench.LoadModelClosedLoop
	}
	if isCompletionLike {
		maxTok, _ := strconv.Atoi(vals[fieldIdx("Max Output Tokens")])
		cfg.MaxOutputTokens = maxTok
		cfg.SystemPrompt = vals[fieldIdx("System Prompt")]
		cfg.TTFTIncludesReasoning = m.ttftReasoningOn
	}
	providers := []bench.ProviderConfig{
		{Name: "Provider", URL: apiURL, APIKey: vals[keyIdx], Model: vals[modelIdx], CustomParams: vals[customIdx]},
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
			focused := i == m.focusIndex
			cursor := "  "
			if focused {
				cursor = "> "
			}
			if m.fieldDefs[i].fieldType == "toggle" {
				toggle := m.renderToggle(m.fieldDefs[i].label)
				sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, toggle))
			} else {
				inp := m.inputs[i].View()
				sb.WriteString(fmt.Sprintf("%s%s  %s\n", cursor, label, inp))
			}
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
	help := "tab/shift+tab navigate  •  ctrl+s start benchmark  •  esc back  •  ctrl+c quit"
	if m.isToggleField() {
		help = "←/→ toggle  •  " + help
	}
	sb.WriteString(helpStyle.Render(help))
	return sb.String()
}

// renderToggle renders the radio buttons for the toggle field with the given label.
func (m configModel) renderToggle(label string) string {
	switch label {
	case "Load Model":
		if m.loadModelIndex == 0 {
			return "(●) closed-loop  ( ) open-loop"
		}
		return "( ) closed-loop  (●) open-loop"
	case "TTFT Includes Reasoning":
		if m.ttftReasoningOn {
			return "(●) yes  ( ) no"
		}
		return "( ) yes  (●) no"
	}
	panic(fmt.Sprintf("no toggle renderer for field %q", label))
}
