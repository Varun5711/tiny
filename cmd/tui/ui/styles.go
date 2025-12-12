package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Golang-themed colors
	Primary   = lipgloss.Color("#00ADD8") // Go cyan
	Secondary = lipgloss.Color("#5DC9E2") // Light cyan
	Accent    = lipgloss.Color("#CE3262") // Go pink/magenta
	Success   = lipgloss.Color("#00D9A5") // Bright teal
	Warning   = lipgloss.Color("#FFB84D") // Warm orange
	Error     = lipgloss.Color("#FF5A87") // Pink error
	Muted     = lipgloss.Color("#6B7B8C") // Muted blue-gray
	Text      = lipgloss.Color("#E3F2FD") // Light blue-white
	BgDark    = lipgloss.Color("#0A1A2F") // Deep navy
	BgLight   = lipgloss.Color("#1A2942") // Navy blue

	TitleStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			Padding(0, 1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(Secondary).
			Padding(0, 1)

	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(1, 2).
			MarginTop(1)

	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(Accent).
				Bold(true).
				PaddingLeft(2)

	ItemStyle = lipgloss.NewStyle().
			Foreground(Text).
			PaddingLeft(2)

	InfoStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Italic(true)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	InputStyle = lipgloss.NewStyle().
			Foreground(Text).
			Border(lipgloss.NormalBorder()).
			BorderForeground(Secondary).
			Padding(0, 1)

	FocusedInputStyle = lipgloss.NewStyle().
				Foreground(Text).
				Border(lipgloss.NormalBorder()).
				BorderForeground(Accent).
				Padding(0, 1)

	ButtonStyle = lipgloss.NewStyle().
			Foreground(Text).
			Background(Primary).
			Padding(0, 2).
			MarginRight(1).
			Bold(true)

	ButtonFocusedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(Accent).
				Padding(0, 2).
				MarginRight(1).
				Bold(true)

	HeaderStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Background(BgLight).
			Padding(1, 2).
			Bold(true).
			Width(120)

	FooterStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Background(BgDark).
			Padding(0, 2)

	StatsStyle = lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true).
			PaddingRight(2)

	LabelStyle = lipgloss.NewStyle().
			Foreground(Secondary).
			Width(20)

	ValueStyle = lipgloss.NewStyle().
			Foreground(Text).
			Bold(true)
)
