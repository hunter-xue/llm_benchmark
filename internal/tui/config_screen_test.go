package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"

	"embedding_benchmark/internal/bench"
)

// setInputsByLabel sets input values by matching field labels.
func setInputsByLabel(m *configModel, values map[string]string) {
	for i, fd := range m.fieldDefs {
		if val, ok := values[fd.label]; ok {
			m.inputs[i] = setTextInputValue(m.inputs[i], val)
		}
	}
}

func setTextInputValue(ti textinput.Model, val string) textinput.Model {
	ti.SetValue(val)
	return ti
}

func fieldLabels(defs []fieldDef) []string {
	labels := make([]string, len(defs))
	for i, d := range defs {
		labels[i] = d.label
	}
	return labels
}

func containsLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

func assertLabelOrder(t *testing.T, defs []fieldDef, expected []string) {
	t.Helper()
	labels := fieldLabels(defs)
	if len(labels) != len(expected) {
		t.Fatalf("field count = %d, want %d; got fields %v", len(labels), len(expected), labels)
	}
	for i, want := range expected {
		if labels[i] != want {
			t.Fatalf("field[%d] = %q, want %q; full list: %v", i, labels[i], want, labels)
		}
	}
}

func TestBuildFieldDefs_CompletionClosedLoop(t *testing.T) {
	defs := buildFieldDefs(bench.ModeCompletion, "single", 0)
	labels := fieldLabels(defs)

	if !containsLabel(labels, "Concurrency") {
		t.Error("closed-loop should include Concurrency field")
	}
	if containsLabel(labels, "Max In-Flight") {
		t.Error("closed-loop should not include Max In-Flight field")
	}
	if containsLabel(labels, "Request Rate") {
		t.Error("closed-loop should not include Request Rate field")
	}
	if !containsLabel(labels, "Load Model") {
		t.Error("completion mode should include Load Model toggle")
	}
	for _, d := range defs {
		if d.label == "Load Model" && d.fieldType != "toggle" {
			t.Error("Load Model field should have fieldType \"toggle\"")
		}
	}
	assertLabelOrder(t, defs, []string{
		"API URL", "API Key", "Model", "Custom Params",
		"Load Model", "Concurrency", "Total Requests", "Input Tokens",
		"Max Output Tokens", "TTFT Includes Reasoning", "System Prompt",
		"Request Logging",
	})
}

func TestBuildFieldDefs_CompletionOpenLoop(t *testing.T) {
	defs := buildFieldDefs(bench.ModeCompletion, "single", 1)
	labels := fieldLabels(defs)

	if containsLabel(labels, "Concurrency") {
		t.Error("open-loop should not include Concurrency field")
	}
	if !containsLabel(labels, "Max In-Flight") {
		t.Error("open-loop should include Max In-Flight field")
	}
	if !containsLabel(labels, "Request Rate") {
		t.Error("open-loop should include Request Rate field")
	}
	if !containsLabel(labels, "Load Model") {
		t.Error("completion mode should include Load Model toggle")
	}
	assertLabelOrder(t, defs, []string{
		"API URL", "API Key", "Model", "Custom Params",
		"Load Model", "Max In-Flight", "Request Rate", "Total Requests", "Input Tokens",
		"Max Output Tokens", "TTFT Includes Reasoning", "System Prompt",
		"Request Logging",
	})
}

func TestBuildFieldDefs_Embedding(t *testing.T) {
	defs := buildFieldDefs(bench.ModeEmbedding, "single", 0)
	labels := fieldLabels(defs)

	if !containsLabel(labels, "Concurrency") {
		t.Error("embedding should include Concurrency")
	}
	if containsLabel(labels, "Load Model") {
		t.Error("embedding should not include Load Model toggle")
	}
	if containsLabel(labels, "Max In-Flight") {
		t.Error("embedding should not include Max In-Flight field")
	}
	if containsLabel(labels, "Request Rate") {
		t.Error("embedding should not include Request Rate field")
	}
}

func TestBuildFieldDefs_AnthropicMessages(t *testing.T) {
	defs := buildFieldDefs(bench.ModeAnthropicMessages, "single", 0)
	labels := fieldLabels(defs)

	if !containsLabel(labels, "Concurrency") {
		t.Error("anthropic should include Concurrency")
	}
	if containsLabel(labels, "Load Model") {
		t.Error("anthropic should not include Load Model toggle")
	}
	if containsLabel(labels, "Max In-Flight") {
		t.Error("anthropic should not include Max In-Flight field")
	}
	if containsLabel(labels, "Request Rate") {
		t.Error("anthropic should not include Request Rate field")
	}
}

func TestValidate_OpenLoopRequiresMaxInFlight(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1
	m.rebuildFields()

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "",
		"Request Rate":   "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, _, err := m.validate()
	if err == nil {
		t.Fatal("expected error when Max In-Flight is empty in open-loop mode")
	}
	if !strings.Contains(err.Error(), "Max In-Flight") {
		t.Errorf("error should mention Max In-Flight, got: %v", err)
	}
}

func TestValidate_OpenLoopRequiresRequestRate(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1
	m.rebuildFields()

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "10",
		"Request Rate":   "",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, _, err := m.validate()
	if err == nil {
		t.Fatal("expected error when Request Rate is empty in open-loop mode")
	}
	if !strings.Contains(err.Error(), "Request Rate") {
		t.Errorf("error should mention Request Rate, got: %v", err)
	}
}

func TestValidate_ClosedLoopIgnoresOpenLoopFields(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	// loadModelIndex defaults to 0 (closed-loop)

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Concurrency":    "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("closed-loop should not require open-loop fields, got: %v", err)
	}
	if cfg.LoadModel != bench.LoadModelClosedLoop {
		t.Errorf("expected LoadModel=closed_loop, got %s", cfg.LoadModel)
	}
	if cfg.Concurrency != 10 {
		t.Errorf("expected Concurrency=10, got %d", cfg.Concurrency)
	}
}

func TestValidate_OpenLoopSetsConfigFields(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	m.loadModelIndex = 1
	m.rebuildFields()

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Max In-Flight":  "5",
		"Request Rate":   "20",
		"Total Requests": "50",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LoadModel != bench.LoadModelOpenLoop {
		t.Errorf("expected LoadModel=open_loop, got %s", cfg.LoadModel)
	}
	if cfg.MaxInFlight != 5 {
		t.Errorf("expected MaxInFlight=5, got %d", cfg.MaxInFlight)
	}
	if cfg.RequestRate != 20 {
		t.Errorf("expected RequestRate=20, got %d", cfg.RequestRate)
	}
	if cfg.TotalRequests != 50 {
		t.Errorf("expected TotalRequests=50, got %d", cfg.TotalRequests)
	}
	if cfg.Concurrency != 5 {
		t.Errorf("expected Concurrency=5 (mirrors MaxInFlight), got %d", cfg.Concurrency)
	}
}

func TestValidate_OpenLoopPKMode(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "pk")
	m.loadModelIndex = 1
	m.rebuildFields()

	setInputsByLabel(&m, map[string]string{
		"Provider A URL":   "https://api.openai.com/v1/chat/completions",
		"Provider A Model": "gpt-4o-mini",
		"Provider B URL":   "https://api.openai.com/v1/chat/completions",
		"Provider B Model": "gpt-4o",
		"Max In-Flight":    "10",
		"Request Rate":     "30",
		"Total Requests":   "20",
		"Input Tokens":     "100",
	})

	providers, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
	if cfg.LoadModel != bench.LoadModelOpenLoop {
		t.Errorf("expected LoadModel=open_loop, got %s", cfg.LoadModel)
	}
	if cfg.MaxInFlight != 10 {
		t.Errorf("expected MaxInFlight=10, got %d", cfg.MaxInFlight)
	}
	if cfg.RequestRate != 30 {
		t.Errorf("expected RequestRate=30, got %d", cfg.RequestRate)
	}
}

func TestValidate_PKCustomParamsRouting(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "pk")

	// Set different Custom Params for A and B
	setInputsByLabel(&m, map[string]string{
		"Provider A URL":   "https://api.openai.com/v1/chat/completions",
		"Provider A Model": "gpt-4o-mini",
		"Provider B URL":   "https://api.openai.com/v1/chat/completions",
		"Provider B Model": "gpt-4o",
		"Concurrency":      "10",
		"Total Requests":   "10",
		"Input Tokens":     "100",
	})

	// Set Custom Params manually by index (since labels are duplicated)
	customAIdx := -1
	customBIdx := -1
	for i, fd := range m.fieldDefs {
		if fd.label == "Custom Params" {
			if customAIdx == -1 {
				customAIdx = i
			} else {
				customBIdx = i
			}
		}
	}
	if customAIdx == -1 || customBIdx == -1 {
		t.Fatal("expected two Custom Params fields in PK mode")
	}
	m.inputs[customAIdx].SetValue(`{"temperature":0.7}`)
	m.inputs[customBIdx].SetValue(`{"top_p":0.9}`)

	providers, _, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if providers[0].CustomParams != `{"temperature":0.7}` {
		t.Errorf("Provider A CustomParams = %q, want %q", providers[0].CustomParams, `{"temperature":0.7}`)
	}
	if providers[1].CustomParams != `{"top_p":0.9}` {
		t.Errorf("Provider B CustomParams = %q, want %q", providers[1].CustomParams, `{"top_p":0.9}`)
	}
}

func TestToggleLoadModel(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	if m.loadModelIndex != 0 {
		t.Errorf("expected default loadModelIndex=0, got %d", m.loadModelIndex)
	}

	m.toggleLoadModel()
	if m.loadModelIndex != 1 {
		t.Errorf("expected loadModelIndex=1 after toggle, got %d", m.loadModelIndex)
	}
	labels := fieldLabels(m.fieldDefs)
	if !containsLabel(labels, "Max In-Flight") {
		t.Error("after toggle to open-loop, should include Max In-Flight")
	}
	if containsLabel(labels, "Concurrency") {
		t.Error("after toggle to open-loop, should not include Concurrency")
	}

	m.toggleLoadModel()
	if m.loadModelIndex != 0 {
		t.Errorf("expected loadModelIndex=0 after second toggle, got %d", m.loadModelIndex)
	}
	labels = fieldLabels(m.fieldDefs)
	if !containsLabel(labels, "Concurrency") {
		t.Error("after toggle back to closed-loop, should include Concurrency")
	}
	if containsLabel(labels, "Max In-Flight") {
		t.Error("after toggle back to closed-loop, should not include Max In-Flight")
	}
}

func TestToggleLoadModel_IgnoredForEmbedding(t *testing.T) {
	m := newConfigModel(bench.ModeEmbedding, "single")
	m.toggleLoadModel() // should be no-op
	if m.loadModelIndex != 0 {
		t.Errorf("toggle should be no-op for embedding, got loadModelIndex=%d", m.loadModelIndex)
	}
	if containsLabel(fieldLabels(m.fieldDefs), "Load Model") {
		t.Error("embedding fields should not include Load Model after toggle attempt")
	}
}

func TestBuildFieldDefs_TTFTReasoningToggleVisibility(t *testing.T) {
	completionDefs := buildFieldDefs(bench.ModeCompletion, "single", 0)
	found := false
	for _, d := range completionDefs {
		if d.label == "TTFT Includes Reasoning" {
			found = true
			if d.fieldType != "toggle" {
				t.Error("TTFT Includes Reasoning should have fieldType \"toggle\"")
			}
		}
	}
	if !found {
		t.Error("completion mode should include TTFT Includes Reasoning toggle")
	}

	anthropicDefs := buildFieldDefs(bench.ModeAnthropicMessages, "single", 0)
	if !containsLabel(fieldLabels(anthropicDefs), "TTFT Includes Reasoning") {
		t.Error("anthropic mode should include TTFT Includes Reasoning toggle")
	}

	embeddingDefs := buildFieldDefs(bench.ModeEmbedding, "single", 0)
	if containsLabel(fieldLabels(embeddingDefs), "TTFT Includes Reasoning") {
		t.Error("embedding mode should not include TTFT Includes Reasoning toggle")
	}

	pkDefs := buildFieldDefs(bench.ModeCompletion, "pk", 0)
	pkFound := false
	for _, d := range pkDefs {
		if d.label == "TTFT Includes Reasoning" {
			pkFound = true
			if d.fieldType != "toggle" {
				t.Error("PK mode: TTFT Includes Reasoning should have fieldType \"toggle\"")
			}
		}
	}
	if !pkFound {
		t.Error("PK completion mode should include TTFT Includes Reasoning toggle")
	}
}

func TestValidate_TTFTIncludesReasoning(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	if !m.ttftReasoningOn {
		t.Error("expected ttftReasoningOn default true")
	}

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Concurrency":    "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.TTFTIncludesReasoning {
		t.Error("expected cfg.TTFTIncludesReasoning=true by default")
	}

	m.ttftReasoningOn = false
	_, cfg, err = m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TTFTIncludesReasoning {
		t.Error("expected cfg.TTFTIncludesReasoning=false after toggle off")
	}
}

func TestToggleFocusedField_Dispatch(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")

	focusByLabel := func(label string) {
		t.Helper()
		for i, fd := range m.fieldDefs {
			if fd.label == label {
				m.focusIndex = i
				return
			}
		}
		t.Fatalf("field %q not found", label)
	}

	focusByLabel("TTFT Includes Reasoning")
	m.toggleFocusedField()
	if m.ttftReasoningOn {
		t.Error("expected ttftReasoningOn=false after toggle")
	}
	if m.loadModelIndex != 0 {
		t.Error("TTFT toggle should not affect loadModelIndex")
	}

	focusByLabel("Load Model")
	m.toggleFocusedField()
	if m.loadModelIndex != 1 {
		t.Error("expected loadModelIndex=1 after Load Model toggle")
	}
	if m.ttftReasoningOn {
		t.Error("Load Model toggle should not affect ttftReasoningOn")
	}
}

func TestBuildFieldDefs_RequestLoggingToggleVisibility(t *testing.T) {
	completionSingle := buildFieldDefs(bench.ModeCompletion, "single", 0)
	found := false
	for _, d := range completionSingle {
		if d.label == "Request Logging" {
			found = true
			if d.fieldType != "toggle" {
				t.Error("Request Logging should have fieldType \"toggle\"")
			}
		}
	}
	if !found {
		t.Error("completion single-provider should include Request Logging toggle")
	}

	completionPK := buildFieldDefs(bench.ModeCompletion, "pk", 0)
	if containsLabel(fieldLabels(completionPK), "Request Logging") {
		t.Error("PK mode should not include Request Logging toggle")
	}

	anthropic := buildFieldDefs(bench.ModeAnthropicMessages, "single", 0)
	if containsLabel(fieldLabels(anthropic), "Request Logging") {
		t.Error("anthropic mode should not include Request Logging toggle")
	}

	embedding := buildFieldDefs(bench.ModeEmbedding, "single", 0)
	if containsLabel(fieldLabels(embedding), "Request Logging") {
		t.Error("embedding mode should not include Request Logging toggle")
	}
}

func TestValidate_RequestLogging(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	if m.requestLoggingOn {
		t.Error("expected requestLoggingOn default false")
	}

	setInputsByLabel(&m, map[string]string{
		"API URL":        "https://api.openai.com/v1/chat/completions",
		"Model":          "gpt-4o-mini",
		"Concurrency":    "10",
		"Total Requests": "10",
		"Input Tokens":   "100",
	})

	_, cfg, err := m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RequestLogging {
		t.Error("expected cfg.RequestLogging=false by default")
	}

	m.requestLoggingOn = true
	_, cfg, err = m.validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.RequestLogging {
		t.Error("expected cfg.RequestLogging=true after toggle on")
	}
}

func TestToggleFocusedField_RequestLoggingDispatch(t *testing.T) {
	m := newConfigModel(bench.ModeCompletion, "single")
	for i, fd := range m.fieldDefs {
		if fd.label == "Request Logging" {
			m.focusIndex = i
			break
		}
	}
	if m.fieldDefs[m.focusIndex].label != "Request Logging" {
		t.Fatal("Request Logging field not found")
	}
	m.toggleFocusedField()
	if !m.requestLoggingOn {
		t.Error("expected requestLoggingOn=true after toggle")
	}
	if m.loadModelIndex != 0 {
		t.Error("Request Logging toggle should not affect loadModelIndex")
	}
	if !m.ttftReasoningOn {
		t.Error("Request Logging toggle should not affect ttftReasoningOn")
	}
}
