package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette and reusable Lipgloss styles for the TUI.
//
// The palette is inspired by Go's brand colors (cyan, pink/magenta) with
// a dark navy background for high contrast in terminal emulators. All views
// share these styles to maintain a consistent look and feel. Changing a
// color here propagates to every screen automatically.
var (
	// Primary palette -- derived from Go's official brand colors.
	Primary   = lipgloss.Color("#00ADD8") // Go cyan  -- titles, borders, primary actions
	Secondary = lipgloss.Color("#5DC9E2") // Light cyan -- labels, secondary text
	Accent    = lipgloss.Color("#CE3262") // Go pink/magenta -- focused items, highlights
	Success   = lipgloss.Color("#00D9A5") // Bright teal -- success messages, user greeting
	Warning   = lipgloss.Color("#FFB84D") // Warm orange -- click counts, expiry warnings
	Error     = lipgloss.Color("#FF5A87") // Pink error -- error messages
	Muted     = lipgloss.Color("#6B7B8C") // Muted blue-gray -- help text, timestamps
	Text      = lipgloss.Color("#E3F2FD") // Light blue-white -- body text
	BgDark    = lipgloss.Color("#0A1A2F") // Deep navy -- status bar background
	BgLight   = lipgloss.Color("#1A2942") // Navy blue -- header background

	// TitleStyle is used for screen headings (e.g., "CREATE SHORT URL").
	TitleStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			Padding(0, 1)

	// SubtitleStyle is used for secondary headings below the main title.
	SubtitleStyle = lipgloss.NewStyle().
			Foreground(Secondary).
			Padding(0, 1)

	// BoxStyle wraps content in a rounded border, used for the menu and
	// form containers.
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(1, 2).
			MarginTop(1)

	// SelectedItemStyle highlights the menu item under the cursor.
	SelectedItemStyle = lipgloss.NewStyle().
				Foreground(Accent).
				Bold(true).
				PaddingLeft(2)

	// ItemStyle is the default (unselected) menu item appearance.
	ItemStyle = lipgloss.NewStyle().
			Foreground(Text).
			PaddingLeft(2)

	// InfoStyle is used for help text and hints at the bottom of screens.
	InfoStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Italic(true)

	// SuccessStyle is used for positive confirmations (e.g., "URL created").
	SuccessStyle = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	// ErrorStyle is used for error messages displayed inline in forms.
	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	// InputStyle is the default (unfocused) text input appearance.
	InputStyle = lipgloss.NewStyle().
			Foreground(Text).
			Border(lipgloss.NormalBorder()).
			BorderForeground(Secondary).
			Padding(0, 1)

	// FocusedInputStyle highlights the currently active text input with
	// an accent-colored border so the user knows which field has focus.
	FocusedInputStyle = lipgloss.NewStyle().
				Foreground(Text).
				Border(lipgloss.NormalBorder()).
				BorderForeground(Accent).
				Padding(0, 1)

	// ButtonStyle is the default button appearance.
	ButtonStyle = lipgloss.NewStyle().
			Foreground(Text).
			Background(Primary).
			Padding(0, 2).
			MarginRight(1).
			Bold(true)

	// ButtonFocusedStyle highlights a button when it has focus.
	ButtonFocusedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(Accent).
				Padding(0, 2).
				MarginRight(1).
				Bold(true)

	// HeaderStyle is used for full-width header bars at the top of screens.
	HeaderStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Background(BgLight).
			Padding(1, 2).
			Bold(true).
			Width(120)

	// FooterStyle is used for status/footer bars at the bottom.
	FooterStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Background(BgDark).
			Padding(0, 2)

	// StatsStyle highlights numeric statistics (click counts, totals).
	StatsStyle = lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true).
			PaddingRight(2)

	// LabelStyle is used for form field labels (fixed width for alignment).
	LabelStyle = lipgloss.NewStyle().
			Foreground(Secondary).
			Width(20)

	// ValueStyle is used for displaying data values alongside labels.
	ValueStyle = lipgloss.NewStyle().
			Foreground(Text).
			Bold(true)
)
