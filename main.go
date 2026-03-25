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
	bpeFile := flag.String("bpe-file", "./cl100k_base.tiktoken", "Path to local cl100k_base.tiktoken file (required for offline token counting)")
	flag.Parse()

	if _, err := os.Stat(*bpeFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: BPE file not found: %s\n", *bpeFile)
		fmt.Fprintf(os.Stderr, "\nThis tool requires the cl100k_base.tiktoken file for token counting.\n")
		fmt.Fprintf(os.Stderr, "Place the file in the same directory as the binary, or specify its path:\n")
		fmt.Fprintf(os.Stderr, "  %s --bpe-file /path/to/cl100k_base.tiktoken\n", os.Args[0])
		os.Exit(1)
	}

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
