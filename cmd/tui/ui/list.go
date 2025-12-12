package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Varun5711/shorternit/cmd/tui/client"
)

type URLItem struct {
	ShortCode string
	ShortURL  string
	LongURL   string
	Clicks    int64
	CreatedAt string
}

type listURLsSuccessMsg struct {
	urls []URLItem
}

type listURLsErrorMsg struct {
	err error
}

type ListModel struct {
	urls    []URLItem
	cursor  int
	loading bool
	err     error
	client  *client.Client
	loaded  bool
}

func (m *ListModel) Init() tea.Cmd {
	return nil
}

func NewListModel() *ListModel {
	return &ListModel{
		urls: []URLItem{},
	}
}

func (m *ListModel) SetClient(c *client.Client) {
	m.client = c
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func listURLsCmd(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.ListURLs(100, 0)
		if err != nil {
			return listURLsErrorMsg{err: err}
		}

		urls := make([]URLItem, 0, len(resp.Urls))
		for _, u := range resp.Urls {
			createdAt := time.Unix(u.CreatedAt, 0)
			ago := time.Since(createdAt)

			var timeStr string
			if ago < time.Hour {
				timeStr = fmt.Sprintf("%d min ago", int(ago.Minutes()))
			} else if ago < 24*time.Hour {
				timeStr = fmt.Sprintf("%d hours ago", int(ago.Hours()))
			} else {
				timeStr = fmt.Sprintf("%d days ago", int(ago.Hours()/24))
			}

			urls = append(urls, URLItem{
				ShortCode: u.ShortCode,
				ShortURL:  u.ShortUrl,
				LongURL:   u.LongUrl,
				Clicks:    u.Clicks,
				CreatedAt: timeStr,
			})
		}

		return listURLsSuccessMsg{urls: urls}
	}
}

func (m *ListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case listURLsSuccessMsg:
		m.loading = false
		m.urls = msg.urls
		m.err = nil
		m.loaded = true
		return m, nil

	case listURLsErrorMsg:
		m.loading = false
		m.err = msg.err
		m.loaded = true
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.urls)-1 {
				m.cursor++
			}
		case "r":
			if !m.loading {
				m.loading = true
				m.err = nil
				return m, listURLsCmd(m.client)
			}
		}
	}

	if !m.loaded && !m.loading && m.client != nil {
		m.loading = true
		return m, listURLsCmd(m.client)
	}

	return m, nil
}

func (m *ListModel) View() string {
	var b strings.Builder

	header := TitleStyle.Render("YOUR URLS")
	b.WriteString(lipgloss.NewStyle().
		Width(80).
		Align(lipgloss.Center).
		MarginTop(2).
		MarginBottom(2).
		Render(header))
	b.WriteString("\n\n")

	if m.loading {
		loading := lipgloss.NewStyle().
			Foreground(Accent).
			Render("⏳ Loading URLs...")
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).MarginTop(2).Render(loading))
		b.WriteString("\n")
	} else if m.err != nil {
		errMsg := ErrorStyle.Render("❌ " + m.err.Error())
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).MarginTop(2).Render(errMsg))
		b.WriteString("\n")
	} else if len(m.urls) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(Muted).
			Render("📝 No URLs found. Create one first!")
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).MarginTop(2).Render(empty))
		b.WriteString("\n")
	} else {
		// Display URLs as cards
		for i, url := range m.urls {
			var cardStyle lipgloss.Style
			if i == m.cursor {
				cardStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(Accent).
					Padding(1, 2).
					Width(70).
					MarginBottom(1)
			} else {
				cardStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(Muted).
					Padding(1, 2).
					Width(70).
					MarginBottom(1)
			}

			// Card content
			shortURLLabel := lipgloss.NewStyle().Foreground(Accent).Bold(true).Render("🔗 Short URL: ")
			shortURLValue := lipgloss.NewStyle().Foreground(Success).Render(url.ShortURL)
			shortURLLine := shortURLLabel + shortURLValue

			longURLLabel := lipgloss.NewStyle().Foreground(Secondary).Render("📎 Original: ")
			longURLValue := lipgloss.NewStyle().Foreground(Text).Render(truncate(url.LongURL, 50))
			longURLLine := longURLLabel + longURLValue

			statsLabel := lipgloss.NewStyle().Foreground(Warning).Render("📊 Clicks: ")
			statsValue := lipgloss.NewStyle().Foreground(Warning).Bold(true).Render(fmt.Sprintf("%d", url.Clicks))
			timeValue := lipgloss.NewStyle().Foreground(Muted).Render(" • Created " + url.CreatedAt)
			statsLine := statsLabel + statsValue + timeValue

			cardContent := lipgloss.JoinVertical(lipgloss.Left,
				shortURLLine,
				longURLLine,
				statsLine,
			)

			card := cardStyle.Render(cardContent)
			b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(card))
		}
	}

	b.WriteString("\n")
	help := InfoStyle.Render("↑/↓ navigate  •  r refresh  •  q back")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(help))

	return BoxStyle.Width(76).Render(b.String())
}
