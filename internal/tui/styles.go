package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			MarginBottom(1)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("212")).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	focusedInputStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("205")).
				Padding(0, 1)

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Width(22)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("33")).
			MarginTop(1).
			MarginBottom(0)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82"))

	winnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("82")).
			Bold(true)

	naStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	borderStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	overlayStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("196")).
			Padding(0, 1)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	panelFocusedBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("33")).
				Padding(0, 1)

	panelBlurredBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238")).
				Padding(0, 1)
)
