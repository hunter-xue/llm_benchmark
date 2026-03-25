package tui

import "github.com/charmbracelet/bubbles/key"

type globalKeyMap struct {
	Quit key.Binding
}

var globalKeys = globalKeyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
}

type navKeyMap struct {
	Back   key.Binding
	Select key.Binding
	Up     key.Binding
	Down   key.Binding
}

var navKeys = navKeyMap{
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
}

type configKeyMap struct {
	Next     key.Binding
	Prev     key.Binding
	Submit   key.Binding
	Back     key.Binding
}

var configKeys = configKeyMap{
	Next:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next field")),
	Prev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev field")),
	Submit: key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "start benchmark")),
	Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

type runningKeyMap struct {
	ShowErrors key.Binding
	Cancel     key.Binding
}

var runningKeys = runningKeyMap{
	ShowErrors: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "view error log")),
	Cancel:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
}

type resultsKeyMap struct {
	Rerun      key.Binding
	ShowErrors key.Binding
	Quit       key.Binding
}

var resultsKeys = resultsKeyMap{
	Rerun:      key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run again")),
	ShowErrors: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "view error log")),
	Quit:       key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
}

type errorViewKeyMap struct {
	Close key.Binding
	Up    key.Binding
	Down  key.Binding
}

var errorViewKeys = errorViewKeyMap{
	Close: key.NewBinding(key.WithKeys("esc", "e"), key.WithHelp("esc/e", "close")),
	Up:    key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:  key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
}

type responseCompareKeyMap struct {
	Close       key.Binding
	SwitchPanel key.Binding
	Up          key.Binding
	Down        key.Binding
	PageUp      key.Binding
	PageDown    key.Binding
}

var responseCompareKeys = responseCompareKeyMap{
	Close:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back to results")),
	SwitchPanel: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch panel")),
	Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up")),
	Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down")),
	PageUp:      key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
	PageDown:    key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
}
