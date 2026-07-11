package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"embedding_benchmark/internal/bench"
	"embedding_benchmark/internal/tui"
)

func main() {
	bpeFile := flag.String("bpe-file", "", "Optional path to an external cl100k_base.tiktoken file")
	flag.Parse()

	tkm, err := bench.InitTiktoken(*bpeFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load tokenizer: %v\n", err)
		os.Exit(1)
	}

	m := tui.NewModel(tkm)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	tui.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		os.Exit(1)
	}
}
