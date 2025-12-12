package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type MenuModel struct {
	cursor   int
	selected int
	items    []string
	userName string
}

func (m *MenuModel) Init() tea.Cmd {
	return nil
}

func NewMenuModel() *MenuModel {
	return &MenuModel{
		cursor:   0,
		selected: -1,
		items: []string{
			"Create Short URL",
			"View URLs",
			"Analytics",
			"Logout",
		},
	}
}

func (m *MenuModel) SetUserName(name string) {
	m.userName = name
}

func (m *MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			m.selected = m.cursor
		}
	}
	return m, nil
}

func (m *MenuModel) View() string {
	var b strings.Builder

	gopher := lipgloss.NewStyle().Foreground(Primary).Render("🐹")
	title := TitleStyle.Render(" TINY ")
	subtitle := SubtitleStyle.Render("URL Shortener - Powered by Go")
	header := gopher + title + gopher + "\n" + subtitle

	b.WriteString(lipgloss.NewStyle().
		Width(120).
		Align(lipgloss.Center).
		MarginTop(2).
		MarginBottom(1).
		Render(header))
	b.WriteString("\n")

	if m.userName != "" {
		greeting := lipgloss.NewStyle().
			Foreground(Success).
			Bold(true).
			Render("👋 Hi, " + m.userName + "!")
		b.WriteString(lipgloss.NewStyle().
			Width(120).
			Align(lipgloss.Center).
			MarginBottom(1).
			Render(greeting))
	}
	b.WriteString("\n")

	var menuItems []string
	for i, item := range m.items {
		cursor := "  "
		style := ItemStyle

		if i == m.cursor {
			cursor = "> "
			style = SelectedItemStyle
		}

		menuItems = append(menuItems, style.Render(cursor+item))
	}

	menu := lipgloss.JoinVertical(lipgloss.Left, menuItems...)
	menuBox := BoxStyle.Width(90).Render(menu)

	b.WriteString(lipgloss.NewStyle().
		Width(120).
		Align(lipgloss.Center).
		Render(menuBox))

	b.WriteString("\n\n")

	help := InfoStyle.Render("↑/↓ navigate  •  enter select  •  q quit")
	b.WriteString(lipgloss.NewStyle().
		Width(120).
		Align(lipgloss.Center).
		Render(help))

	return lipgloss.NewStyle().
		Width(120).
		Height(20).
		Align(lipgloss.Center, lipgloss.Center).
		Render(b.String())
}
