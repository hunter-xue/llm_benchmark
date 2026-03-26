package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	tiktoken "github.com/pkoukk/tiktoken-go"

	"embedding_benchmark/internal/bench"
)

type Screen int

const (
	ScreenModeSelect Screen = iota
	ScreenTestModeSelect
	ScreenConfig
	ScreenRunning
	ScreenResults
	ScreenResponseCompareConfig
	ScreenResponseCompare
)

// prog holds the global program reference so goroutines can call p.Send().
// Safe for single-program use.
var prog *tea.Program

// SetProgram stores the program reference before p.Run() is called.
func SetProgram(p *tea.Program) { prog = p }

// Model is the root TUI model.
type Model struct {
	screen Screen
	width  int
	height int

	// Tiktoken (initialized before TUI starts)
	tkm *tiktoken.Tiktoken

	// Shared state determined during navigation
	apiMode  string // "embedding" or "completion"
	testMode string // "single" or "pk"

	// Current benchmark config (set when leaving config screen)
	providers []bench.ProviderConfig
	cfg       bench.BenchConfig

	// Cancel function for in-progress benchmarks
	cancelFunc context.CancelFunc

	// Sub-models
	modeSelect     modeSelectModel
	testModeSelect testModeSelectModel
	config         configModel
	running        runningModel
	results        resultsModel
	errorViewport  errorViewportModel
	showErrors     bool

	// Response compare standalone flow
	compareConfig   compareConfigModel
	responseCompare responseCompareModel
	compareDone     int

	// Track how many provider benchmarks have finished (for PK mode)
	doneProviders int
}

// NewModel creates the initial TUI model.
func NewModel(tkm *tiktoken.Tiktoken) Model {
	return Model{
		tkm:        tkm,
		screen:     ScreenModeSelect,
		modeSelect: newModeSelect(),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.errorViewport.setSize(m.width-4, m.height-4)
		m.running.setWidth(m.width)
		m.results.setWidth(m.width)
		m.config.setWidth(m.width)
		m.compareConfig.setWidth(m.width)
		m.responseCompare.setSize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		// Global quit
		if msg.String() == "ctrl+c" {
			if m.cancelFunc != nil {
				m.cancelFunc()
			}
			return m, tea.Quit
		}

		// Error overlay captures all keys
		if m.showErrors {
			switch msg.String() {
			case "esc", "e":
				m.showErrors = false
				return m, nil
			}
			var cmd tea.Cmd
			m.errorViewport, cmd = m.errorViewport.update(msg)
			return m, cmd
		}

		// Per-screen key handling
		return m.handleKey(msg)

	case ProgressMsg:
		var cmd tea.Cmd
		m.running, cmd = m.running.update(msg)
		return m, cmd

	case BenchDoneMsg:
		// Accumulate result
		if msg.EmbeddingReport != nil {
			m.results.addEmbeddingResult(msg.ProviderIndex, msg.EmbeddingReport)
		}
		if msg.CompletionReport != nil {
			m.results.addCompletionResult(msg.ProviderIndex, msg.CompletionReport)
		}
		m.doneProviders++

		// Refresh error viewport content
		m.errorViewport.setContent(m.results.mergedErrorDetails())

		if m.doneProviders >= len(m.providers) {
			// All done — go to results
			m.screen = ScreenResults
		}
		return m, nil

	case CompareResponseMsg:
		m.responseCompare.setResponse(msg.ProviderIndex, msg.Body, msg.Err)
		m.compareDone++
		return m, nil

	case spinner.TickMsg:
		if m.screen == ScreenRunning {
			var cmd tea.Cmd
			m.running, cmd = m.running.update(msg)
			return m, cmd
		}
		if m.screen == ScreenResponseCompare {
			var cmd tea.Cmd
			m.responseCompare, cmd = m.responseCompare.update(msg)
			return m, cmd
		}
		return m, nil
	}

	// Delegate to active screen for component messages (progress frame, etc.)
	if m.screen == ScreenRunning {
		var cmd tea.Cmd
		m.running, cmd = m.running.update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {

	case ScreenModeSelect:
		switch msg.String() {
		case "enter":
			m.apiMode = m.modeSelect.selected()
			m.testModeSelect = newTestModeSelect(m.apiMode)
			m.screen = ScreenTestModeSelect
			return m, nil
		default:
			var cmd tea.Cmd
			m.modeSelect, cmd = m.modeSelect.update(msg)
			return m, cmd
		}

	case ScreenTestModeSelect:
		switch msg.String() {
		case "esc":
			m.screen = ScreenModeSelect
			return m, nil
		case "enter":
			m.testMode = m.testModeSelect.selected()
			if m.testMode == "response_compare" {
				m.compareConfig = newCompareConfig()
				m.compareConfig.setWidth(m.width)
				m.screen = ScreenResponseCompareConfig
				return m, m.compareConfig.inputs[0].Focus()
			}
			m.config = newConfigModel(m.apiMode, m.testMode)
			m.screen = ScreenConfig
			return m, m.config.inputs[0].Focus()
		default:
			var cmd tea.Cmd
			m.testModeSelect, cmd = m.testModeSelect.update(msg)
			return m, cmd
		}

	case ScreenConfig:
		switch msg.String() {
		case "esc":
			m.screen = ScreenTestModeSelect
			return m, nil
		case "ctrl+s":
			providers, cfg, err := m.config.validate()
			if err != nil {
				m.config.err = err.Error()
				return m, nil
			}
			m.config.err = ""
			m.providers = providers
			m.cfg = cfg
			return m.startRunning()
		default:
			var cmd tea.Cmd
			m.config, cmd = m.config.update(msg)
			return m, cmd
		}

	case ScreenRunning:
		switch msg.String() {
		case "e":
			if m.running.hasErrors {
				m.showErrors = true
				m.errorViewport.setSize(m.width-4, m.height-4)
			}
			return m, nil
		case "esc":
			if m.cancelFunc != nil {
				m.cancelFunc()
				m.cancelFunc = nil
			}
			// Go back to config
			m.screen = ScreenConfig
			return m, nil
		}
		var cmd tea.Cmd
		m.running, cmd = m.running.update(msg)
		return m, cmd

	case ScreenResults:
		switch msg.String() {
		case "esc":
			m.screen = ScreenTestModeSelect
			return m, nil
		case "r":
			// Rerun: go back to config screen
			m.screen = ScreenConfig
			m.doneProviders = 0
			m.results = newResultsModel(m.apiMode, m.testMode, providerNames(m.providers))
			return m, nil
		case "e":
			if m.results.hasErrors {
				m.showErrors = true
				m.errorViewport.setSize(m.width-4, m.height-4)
			}
			return m, nil
		}

	case ScreenResponseCompareConfig:
		switch msg.String() {
		case "esc":
			m.screen = ScreenTestModeSelect
			return m, nil
		case "ctrl+s":
			res, err := m.compareConfig.validate()
			if err != nil {
				m.compareConfig.err = err.Error()
				return m, nil
			}
			m.compareConfig.err = ""
			m.responseCompare = newResponseCompare(res.providerA.Name, res.providerB.Name)
			m.responseCompare.setSize(m.width, m.height)
			m.compareDone = 0
			m.screen = ScreenResponseCompare
			return m, tea.Batch(
				m.responseCompare.spinner.Tick,
				startCompareRequestsDirect(prog,
					res.providerA, res.providerB,
					res.userMessage, res.systemPrompt,
					res.customParamsA, res.customParamsB,
				),
			)
		default:
			var cmd tea.Cmd
			m.compareConfig, cmd = m.compareConfig.update(msg)
			return m, cmd
		}

	case ScreenResponseCompare:
		switch msg.String() {
		case "esc":
			m.screen = ScreenResponseCompareConfig
			return m, nil
		case "tab":
			m.responseCompare.switchFocus()
			return m, nil
		default:
			var cmd tea.Cmd
			m.responseCompare, cmd = m.responseCompare.update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) startRunning() (tea.Model, tea.Cmd) {
	m.doneProviders = 0
	m.running = newRunningModel(m.providers, m.cfg)
	m.running.setWidth(m.width)
	m.results = newResultsModel(m.apiMode, m.testMode, providerNames(m.providers))
	m.errorViewport = newErrorViewport()
	m.errorViewport.setSize(m.width-4, m.height-4)
	m.showErrors = false
	m.screen = ScreenRunning

	cmd, cancel := startBench(prog, m.providers, m.cfg, m.tkm)
	m.cancelFunc = cancel

	return m, tea.Batch(m.running.spinner.Tick, cmd)
}

func providerNames(providers []bench.ProviderConfig) []string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
	}
	return names
}

func (m Model) View() string {
	var content string
	switch m.screen {
	case ScreenModeSelect:
		content = m.modeSelect.view(m.width, m.height)
	case ScreenTestModeSelect:
		content = m.testModeSelect.view(m.width, m.height)
	case ScreenConfig:
		content = m.config.view(m.width, m.height)
	case ScreenRunning:
		content = m.running.view(m.width, m.height)
	case ScreenResults:
		content = m.results.view(m.width, m.height)
	case ScreenResponseCompareConfig:
		content = m.compareConfig.view(m.width, m.height)
	case ScreenResponseCompare:
		content = m.responseCompare.view(m.width, m.height)
	default:
		content = "Loading..."
	}

	if m.showErrors && m.errorViewport.ready {
		// Overlay: show error viewport centered over content
		overlay := m.errorViewport.view()
		return overlayOnTop(content, overlay, m.width, m.height)
	}
	return content
}

// overlayOnTop renders the overlay centered on top of the base content.
func overlayOnTop(base, overlay string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Simple approach: show overlay below title (don't try to pixel-perfect center)
	var sb strings.Builder
	// Show first 2 lines of base then overlay
	shown := 0
	for i, line := range baseLines {
		if i < 2 {
			sb.WriteString(line)
			sb.WriteString("\n")
			shown++
		} else {
			break
		}
	}
	for _, line := range overlayLines {
		sb.WriteString(line)
		sb.WriteString("\n")
		shown++
	}
	// Pad remaining
	remaining := height - shown
	for remaining > 0 {
		sb.WriteString(fmt.Sprintf("%s\n", dimStyle.Render(strings.Repeat(" ", width))))
		remaining--
	}
	return sb.String()
}
