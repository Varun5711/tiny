package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	Primary   = lipgloss.Color("#7C3AED")
	Secondary = lipgloss.Color("#A78BFA")
	Accent    = lipgloss.Color("#60A5FA")
	Success   = lipgloss.Color("#34D399")
	Warning   = lipgloss.Color("#FBBF24")
	Error     = lipgloss.Color("#F87171")
	Muted     = lipgloss.Color("#9CA3AF")
	Text      = lipgloss.Color("#F3F4F6")
	BgDark    = lipgloss.Color("#1F2937")
	BgLight   = lipgloss.Color("#374151")

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
			Width(80)

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
