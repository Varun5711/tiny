package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/cmd/tui/client"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type URLItem struct {
	ShortCode string
	ShortURL  string
	LongURL   string
	Clicks    int64
	CreatedAt string
	ExpiresIn string
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
	page    int
	perPage int
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
		urls:    []URLItem{},
		perPage: 3,
		page:    0,
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

			var expiresStr string
			if u.ExpiresAt > 0 {
				expiresAt := time.Unix(u.ExpiresAt, 0)
				untilExpiry := time.Until(expiresAt)

				if untilExpiry < 0 {
					expiresStr = "Expired"
				} else if untilExpiry < time.Hour {
					expiresStr = fmt.Sprintf("%d min", int(untilExpiry.Minutes()))
				} else if untilExpiry < 24*time.Hour {
					expiresStr = fmt.Sprintf("%d hours", int(untilExpiry.Hours()))
				} else {
					expiresStr = fmt.Sprintf("%d days", int(untilExpiry.Hours()/24))
				}
			} else {
				expiresStr = "Never"
			}

			urls = append(urls, URLItem{
				ShortCode: u.ShortCode,
				ShortURL:  u.ShortUrl,
				LongURL:   u.LongUrl,
				Clicks:    u.Clicks,
				CreatedAt: timeStr,
				ExpiresIn: expiresStr,
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
		totalPages := (len(m.urls) + m.perPage - 1) / m.perPage
		switch msg.String() {
		case "left", "h":
			if m.page > 0 {
				m.page--
				m.cursor = 0
			}
		case "right", "l":
			if m.page < totalPages-1 {
				m.page++
				m.cursor = 0
			}
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			start := m.page * m.perPage
			end := min(start+m.perPage, len(m.urls))
			pageSize := end - start
			if m.cursor < pageSize-1 {
				m.cursor++
			}
		case "r":
			if !m.loading {
				m.loading = true
				m.err = nil
				m.page = 0
				m.cursor = 0
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

	icon := lipgloss.NewStyle().Foreground(Accent).Render("📋")
	header := icon + " " + TitleStyle.Render("YOUR URLS") + " " + icon
	b.WriteString(lipgloss.NewStyle().
		Width(120).
		Align(lipgloss.Center).
		MarginTop(2).
		MarginBottom(2).
		Render(header))
	b.WriteString("\n\n")

	if m.loading {
		loading := lipgloss.NewStyle().
			Foreground(Accent).
			Render("⏳ Loading URLs...")
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).MarginTop(2).Render(loading))
		b.WriteString("\n")
	} else if m.err != nil {
		errMsg := ErrorStyle.Render("❌ " + m.err.Error())
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).MarginTop(2).Render(errMsg))
		b.WriteString("\n")
	} else if len(m.urls) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(Muted).
			Render("📝 No URLs found. Create one first!")
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).MarginTop(2).Render(empty))
		b.WriteString("\n")
	} else {

		start := m.page * m.perPage
		end := min(start+m.perPage, len(m.urls))

		for i := start; i < end; i++ {
			url := m.urls[i]
			relativeIndex := i - start

			var cardStyle lipgloss.Style
			if relativeIndex == m.cursor {
				cardStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(Accent).
					Padding(1, 2).
					Width(100).
					MarginBottom(1)
			} else {
				cardStyle = lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(Muted).
					Padding(1, 2).
					Width(100).
					MarginBottom(1)
			}

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

			expiresLabel := lipgloss.NewStyle().Foreground(Secondary).Render("⏰ Expires: ")
			var expiresValue string
			if url.ExpiresIn == "Expired" {
				expiresValue = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true).Render(url.ExpiresIn)
			} else if url.ExpiresIn == "Never" {
				expiresValue = lipgloss.NewStyle().Foreground(Success).Render(url.ExpiresIn)
			} else {
				expiresValue = lipgloss.NewStyle().Foreground(Warning).Render("in " + url.ExpiresIn)
			}
			expiresLine := expiresLabel + expiresValue

			cardContent := lipgloss.JoinVertical(lipgloss.Left,
				shortURLLine,
				longURLLine,
				statsLine,
				expiresLine,
			)

			card := cardStyle.Render(cardContent)
			b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(card))
		}

		b.WriteString("\n")
		totalPages := (len(m.urls) + m.perPage - 1) / m.perPage
		if totalPages == 0 {
			totalPages = 1
		}
		pagination := InfoStyle.Render(fmt.Sprintf("Page %d/%d  •  Showing %d-%d of %d URLs",
			m.page+1, totalPages, start+1, end, len(m.urls)))
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(pagination))
	}

	b.WriteString("\n")
	help := InfoStyle.Render("↑/↓ navigate  •  ←/→ page  •  r refresh  •  q back")
	b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(help))

	return BoxStyle.Width(116).Render(b.String())
}
