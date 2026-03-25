package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	tiktoken "github.com/pkoukk/tiktoken-go"

	"embedding_benchmark/internal/bench"
)

type providerProgress struct {
	name      string
	completed int
	errors    int
	total     int
	bar       progress.Model
}

type runningModel struct {
	spinner    spinner.Model
	providers  []providerProgress
	doneCount  int
	totalDone  int // how many providers completed
	testMode   string
	apiMode    string
	hasErrors  bool
	cancelFunc context.CancelFunc

	width int
}

func newRunningModel(providers []bench.ProviderConfig, cfg bench.BenchConfig) runningModel {
	s := spinner.New()
	s.Spinner = spinner.Dot

	pp := make([]providerProgress, len(providers))
	for i, p := range providers {
		bar := progress.New(progress.WithDefaultGradient())
		pp[i] = providerProgress{
			name:  p.Name,
			total: cfg.TotalRequests,
			bar:   bar,
		}
	}

	return runningModel{
		spinner:   s,
		providers: pp,
		testMode:  cfg.Mode,
		apiMode:   cfg.Mode,
	}
}

// startBench returns a tea.Cmd that launches the benchmark goroutine(s).
func startBench(
	p *tea.Program,
	providers []bench.ProviderConfig,
	cfg bench.BenchConfig,
	tkm *tiktoken.Tiktoken,
) (tea.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	testText, actualTokens, err := prepareBenchText(cfg, tkm)
	if err != nil {
		// If we can't prepare text, immediately return done with error
		cancel()
		return func() tea.Msg {
			errMsg := fmt.Sprintf("failed to prepare benchmark text: %v", err)
			if cfg.Mode == bench.ModeEmbedding {
				r := bench.EmbeddingReport{ErrorDetails: map[string]int{errMsg: 1}}
				return BenchDoneMsg{ProviderIndex: 0, EmbeddingReport: &r}
			}
			r := bench.CompletionReport{ErrorDetails: map[string]int{errMsg: 1}}
			return BenchDoneMsg{ProviderIndex: 0, CompletionReport: &r}
		}, cancel
	}

	for i, prov := range providers {
		idx := i
		pv := prov
		go func() {
			onProgress := func(completed, errors int) {
				p.Send(ProgressMsg{
					ProviderIndex: idx,
					Completed:     completed,
					TotalErrors:   errors,
				})
			}

			if cfg.Mode == bench.ModeEmbedding {
				r := bench.RunEmbeddingBench(ctx, pv, cfg, testText, actualTokens, onProgress)
				p.Send(BenchDoneMsg{ProviderIndex: idx, EmbeddingReport: &r})
			} else {
				r := bench.RunCompletionBench(ctx, pv, cfg, testText, actualTokens, tkm, onProgress)
				p.Send(BenchDoneMsg{ProviderIndex: idx, CompletionReport: &r})
			}
		}()
	}

	return nil, cancel
}

// startCompareRequestsDirect launches one non-streaming request per provider using
// the given user message directly (no benchmark text generation needed).
func startCompareRequestsDirect(
	p *tea.Program,
	providerA, providerB bench.ProviderConfig,
	userMessage, systemPrompt string,
) tea.Cmd {
	return func() tea.Msg {
		cfg := bench.BenchConfig{
			Mode:         bench.ModeCompletion,
			SystemPrompt: systemPrompt,
		}
		for i, prov := range []bench.ProviderConfig{providerA, providerB} {
			idx := i
			pv := prov
			go func() {
				body, err := bench.DoCompareRequest(context.Background(), pv, cfg, userMessage)
				p.Send(CompareResponseMsg{ProviderIndex: idx, Body: body, Err: err})
			}()
		}
		return nil
	}
}

func prepareBenchText(cfg bench.BenchConfig, tkm *tiktoken.Tiktoken) (string, int, error) {
	text, err := bench.GenerateTextByTokens(tkm, cfg.TargetTokens)
	if err != nil {
		return "", 0, err
	}
	actualTokens := len(tkm.EncodeOrdinary(text))
	return text, actualTokens, nil
}

func (m runningModel) update(msg tea.Msg) (runningModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case ProgressMsg:
		if msg.ProviderIndex < len(m.providers) {
			m.providers[msg.ProviderIndex].completed = msg.Completed
			m.providers[msg.ProviderIndex].errors = msg.TotalErrors
			if msg.TotalErrors > 0 {
				m.hasErrors = true
			}
		}
		var cmds []tea.Cmd
		for i := range m.providers {
			pct := 0.0
			total := m.providers[i].total
			done := m.providers[i].completed + m.providers[i].errors
			if total > 0 {
				pct = float64(done) / float64(total)
			}
			cmd := m.providers[i].bar.SetPercent(pct)
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case progress.FrameMsg:
		var cmds []tea.Cmd
		for i := range m.providers {
			newBar, cmd := m.providers[i].bar.Update(msg)
			if b, ok := newBar.(progress.Model); ok {
				m.providers[i].bar = b
			}
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m *runningModel) setWidth(w int) {
	m.width = w
	barW := w - 30
	if barW < 20 {
		barW = 20
	}
	for i := range m.providers {
		m.providers[i].bar.Width = barW
	}
}

func (m runningModel) view(width, height int) string {
	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Benchmark Running"))
	sb.WriteString("\n\n")
	sb.WriteString(m.spinner.View() + " Running benchmark...\n\n")

	for _, pp := range m.providers {
		name := pp.name
		if len(m.providers) > 1 {
			sb.WriteString(sectionStyle.Render(name) + "\n")
		}
		done := pp.completed + pp.errors
		sb.WriteString(fmt.Sprintf("  Completed: %d/%d  |  Errors: %d\n",
			done, pp.total, pp.errors))
		sb.WriteString("  " + pp.bar.View() + "\n\n")
	}

	if m.hasErrors {
		sb.WriteString(errorStyle.Render("  Errors detected during test."))
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render("  Press 'e' to view error log"))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("esc cancel  •  ctrl+c quit"))
	return sb.String()
}
