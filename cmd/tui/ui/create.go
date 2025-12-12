package ui

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Varun5711/shorternit/cmd/tui/client"
	pb "github.com/Varun5711/shorternit/proto/url"
)

type createURLSuccessMsg struct {
	shortURL string
	qrCode   string
}

type createURLErrorMsg struct {
	err error
}

type CreateModel struct {
	urlInput     string
	aliasInput   string
	focusedInput int
	loading      bool
	result       string
	err          error
	client       *client.Client
}

func (m *CreateModel) Init() tea.Cmd {
	return nil
}

func NewCreateModel() *CreateModel {
	return &CreateModel{
		focusedInput: 0,
	}
}

func (m *CreateModel) SetClient(c *client.Client) {
	m.client = c
}

func validateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must include a domain (e.g., google.com)")
	}

	return nil
}

func validateAlias(alias string) error {
	if alias == "" {
		return nil
	}

	if len(alias) < 3 {
		return fmt.Errorf("alias must be at least 3 characters")
	}

	if len(alias) > 50 {
		return fmt.Errorf("alias must be less than 50 characters")
	}

	if !strings.HasPrefix(alias, "/") {
		for _, char := range alias {
			if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_') {
				return fmt.Errorf("alias can only contain letters, numbers, hyphens, and underscores")
			}
		}
	}

	return nil
}

func createURLCmd(c *client.Client, longURL, alias string) tea.Cmd {
	return func() tea.Msg {
		expiresAt := time.Now().Add(3 * 24 * time.Hour).Unix()

		var resp interface{}
		var err error

		if alias != "" {
			resp, err = c.CreateCustomURL(alias, longURL, expiresAt)
		} else {
			resp, err = c.CreateURL(longURL, expiresAt)
		}

		if err != nil {
			return createURLErrorMsg{err: err}
		}

		var shortURL, qrCode string
		switch r := resp.(type) {
		case *pb.CreateURLResponse:
			shortURL = r.ShortUrl
			qrCode = r.QrCode
		case *pb.CreateCustomURLResponse:
			shortURL = r.ShortUrl
			qrCode = r.QrCode
		}

		return createURLSuccessMsg{
			shortURL: shortURL,
			qrCode:   qrCode,
		}
	}
}

func (m *CreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case createURLSuccessMsg:
		m.loading = false
		m.result = msg.shortURL
		m.err = nil
		return m, nil

	case createURLErrorMsg:
		m.loading = false
		m.err = msg.err
		m.result = ""
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch msg.String() {
		case "tab":
			m.focusedInput = (m.focusedInput + 1) % 2
		case "enter":
			if err := validateURL(m.urlInput); err != nil {
				m.err = err
				return m, nil
			}

			if err := validateAlias(m.aliasInput); err != nil {
				m.err = err
				return m, nil
			}

			if m.client != nil {
				m.loading = true
				m.err = nil
				m.result = ""
				return m, createURLCmd(m.client, m.urlInput, m.aliasInput)
			} else {
				m.err = fmt.Errorf("client not connected")
			}
		case "backspace":
			if m.focusedInput == 0 && len(m.urlInput) > 0 {
				m.urlInput = m.urlInput[:len(m.urlInput)-1]
			} else if m.focusedInput == 1 && len(m.aliasInput) > 0 {
				m.aliasInput = m.aliasInput[:len(m.aliasInput)-1]
			}
		case "ctrl+l":
			m.urlInput = ""
			m.aliasInput = ""
			m.result = ""
			m.err = nil
		default:
			if len(msg.String()) == 1 {
				if m.focusedInput == 0 {
					m.urlInput += msg.String()
				} else {
					m.aliasInput += msg.String()
				}
			}
		}
	}
	return m, nil
}

func (m *CreateModel) View() string {
	var b strings.Builder

	header := TitleStyle.Render("CREATE SHORT URL")
	b.WriteString(lipgloss.NewStyle().
		Width(80).
		Align(lipgloss.Center).
		MarginTop(2).
		MarginBottom(2).
		Render(header))
	b.WriteString("\n\n")

	urlLabel := LabelStyle.Render("Long URL:")
	var urlInputStyle lipgloss.Style
	if m.focusedInput == 0 {
		urlInputStyle = FocusedInputStyle
	} else {
		urlInputStyle = InputStyle
	}
	urlValue := urlInputStyle.Width(50).Render(m.urlInput)
	urlField := lipgloss.JoinHorizontal(lipgloss.Left, urlLabel, urlValue)
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(urlField))
	b.WriteString("\n\n")

	aliasLabel := LabelStyle.Render("Custom Alias:")
	aliasHint := InfoStyle.Render(" (optional)")
	var aliasInputStyle lipgloss.Style
	if m.focusedInput == 1 {
		aliasInputStyle = FocusedInputStyle
	} else {
		aliasInputStyle = InputStyle
	}
	aliasValue := aliasInputStyle.Width(50).Render(m.aliasInput)
	aliasField := lipgloss.JoinHorizontal(lipgloss.Left, aliasLabel, aliasValue, aliasHint)
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(aliasField))
	b.WriteString("\n\n")

	if m.loading {
		loading := InfoStyle.Render("Creating short URL...")
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(loading))
		b.WriteString("\n")
	}

	if m.result != "" {
		result := SuccessStyle.Render(fmt.Sprintf("Short URL created: %s", m.result))
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(result))
		b.WriteString("\n")
	}

	if m.err != nil {
		errMsg := ErrorStyle.Render("Error: " + m.err.Error())
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(errMsg))
		b.WriteString("\n")
	}

	help := InfoStyle.Render("tab switch  •  enter submit  •  ctrl+l clear  •  q back")
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(help))

	return BoxStyle.Width(76).Render(b.String())
}
