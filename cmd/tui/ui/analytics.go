package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Varun5711/shorternit/cmd/tui/client"
)

type ClickEvent struct {
	EventID        string `json:"event_id"`
	ShortCode      string `json:"short_code"`
	OriginalURL    string `json:"original_url"`
	ClickedAt      string `json:"clicked_at"`
	IPAddress      string `json:"ip_address"`
	Country        string `json:"country"`
	Region         string `json:"region"`
	City           string `json:"city"`
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS             string `json:"os"`
	OSVersion      string `json:"os_version"`
	DeviceType     string `json:"device_type"`
	Referer        string `json:"referer"`
}

type clickEventsSuccessMsg struct {
	events []ClickEvent
}

type clickEventsErrorMsg struct {
	err error
}

type AnalyticsModel struct {
	events  []ClickEvent
	cursor  int
	loading bool
	err     error
	client  *client.Client
	loaded  bool
	token   string
}

func NewAnalyticsModel() *AnalyticsModel {
	return &AnalyticsModel{}
}

func (m *AnalyticsModel) SetClient(c *client.Client) {
	m.client = c
}

func (m *AnalyticsModel) SetToken(token string) {
	m.token = token
}

func fetchClickEventsCmd(token string) tea.Cmd {
	return func() tea.Msg {
		// Call API gateway analytics endpoint
		req, err := http.NewRequest("GET", "http://localhost:8080/api/analytics/clicks?limit=50", nil)
		if err != nil {
			return clickEventsErrorMsg{err: err}
		}

		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return clickEventsErrorMsg{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return clickEventsErrorMsg{err: fmt.Errorf("API error: %s", string(body))}
		}

		var result struct {
			Clicks []ClickEvent `json:"clicks"`
			Total  int          `json:"total"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return clickEventsErrorMsg{err: err}
		}

		return clickEventsSuccessMsg{events: result.Clicks}
	}
}

func (m *AnalyticsModel) Init() tea.Cmd {
	return nil
}

func (m *AnalyticsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clickEventsSuccessMsg:
		m.loading = false
		m.events = msg.events
		m.err = nil
		m.loaded = true
		return m, nil

	case clickEventsErrorMsg:
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
			if m.cursor < len(m.events)-1 {
				m.cursor++
			}
		case "r":
			if !m.loading && m.token != "" {
				m.loading = true
				m.err = nil
				return m, fetchClickEventsCmd(m.token)
			}
		}
	}

	if !m.loaded && !m.loading && m.token != "" {
		m.loading = true
		return m, fetchClickEventsCmd(m.token)
	}

	return m, nil
}

func (m *AnalyticsModel) View() string {
	var b strings.Builder

	header := TitleStyle.Render("📊 CLICK ANALYTICS")
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
			Render("⏳ Loading click events...")
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).MarginTop(2).Render(loading))
		b.WriteString("\n")
	} else if m.err != nil {
		errMsg := ErrorStyle.Render("❌ " + m.err.Error())
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).MarginTop(2).Render(errMsg))
		b.WriteString("\n")
	} else if len(m.events) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(Muted).
			Render("📭 No click events found. Start sharing your links!")
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).MarginTop(2).Render(empty))
		b.WriteString("\n")
	} else {
		// Table header
		headerStyle := lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true).
			Padding(0, 1)

		tableHeader := lipgloss.JoinHorizontal(lipgloss.Left,
			headerStyle.Width(18).Render("IP Address"),
			headerStyle.Width(14).Render("Short Code"),
			headerStyle.Width(20).Render("Clicked At"),
			headerStyle.Width(20).Render("Location"),
			headerStyle.Width(18).Render("Browser"),
			headerStyle.Width(15).Render("Device"),
		)

		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Left).MarginLeft(2).Render(tableHeader))
		b.WriteString("\n")

		separator := lipgloss.NewStyle().
			Foreground(Muted).
			Render(strings.Repeat("─", 116))
		b.WriteString(lipgloss.NewStyle().MarginLeft(2).Render(separator))
		b.WriteString("\n")

		// Table rows
		for i, event := range m.events {
			if i >= 10 { // Show only first 10
				break
			}

			rowStyle := lipgloss.NewStyle().Padding(0, 1)
			if i == m.cursor {
				rowStyle = rowStyle.Foreground(Accent).Bold(true)
			} else {
				rowStyle = rowStyle.Foreground(Text)
			}

			// Format location
			location := event.City
			if location == "" {
				location = event.Country
			} else if event.Country != "" {
				location = location + ", " + event.Country
			}
			if location == "" {
				location = "Unknown"
			}

			// Format browser
			browser := event.Browser
			if browser == "" {
				browser = "Unknown"
			}

			// Format device
			device := event.DeviceType
			if device == "" {
				device = "Unknown"
			}

			row := lipgloss.JoinHorizontal(lipgloss.Left,
				rowStyle.Width(18).Render(truncate(event.IPAddress, 16)),
				rowStyle.Width(14).Render(truncate(event.ShortCode, 12)),
				rowStyle.Width(20).Render(event.ClickedAt),
				rowStyle.Width(20).Render(truncate(location, 18)),
				rowStyle.Width(18).Render(truncate(browser, 16)),
				rowStyle.Width(15).Render(truncate(device, 13)),
			)

			b.WriteString(lipgloss.NewStyle().MarginLeft(2).Render(row))
			b.WriteString("\n")
		}

		// Summary
		b.WriteString("\n")
		summary := InfoStyle.Render(fmt.Sprintf("Showing %d of %d total click events", min(10, len(m.events)), len(m.events)))
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(summary))
	}

	b.WriteString("\n\n")
	help := InfoStyle.Render("↑/↓ navigate  •  r refresh  •  q back")
	b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(help))

	return BoxStyle.Width(116).Render(b.String())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
