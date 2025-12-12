package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Varun5711/shorternit/cmd/tui/client"
)

type loginSuccessMsg struct {
	token  string
	userID string
	email  string
	name   string
}

type loginErrorMsg struct {
	err error
}

type LoginModel struct {
	emailInput    string
	passwordInput string
	focusedInput  int
	loading       bool
	err           error
	authClient    *client.AuthClient
}

func NewLoginModel() *LoginModel {
	return &LoginModel{
		focusedInput: 0,
	}
}

func (m *LoginModel) SetAuthClient(c *client.AuthClient) {
	m.authClient = c
}

func (m *LoginModel) Init() tea.Cmd {
	return nil
}

func loginCmd(c *client.AuthClient, email, password string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.Login(email, password)
		if err != nil {
			return loginErrorMsg{err: err}
		}

		return loginSuccessMsg{
			token:  resp.Token,
			userID: resp.UserId,
			email:  resp.Email,
			name:   resp.Name,
		}
	}
}

func (m *LoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loginSuccessMsg:
		m.loading = false
		m.err = nil
		return m, nil

	case loginErrorMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch msg.String() {
		case "tab", "shift+tab":
			m.focusedInput = (m.focusedInput + 1) % 2
		case "enter":
			if m.emailInput == "" {
				m.err = fmt.Errorf("email cannot be empty")
				return m, nil
			}
			if m.passwordInput == "" {
				m.err = fmt.Errorf("password cannot be empty")
				return m, nil
			}

			if m.authClient != nil {
				m.loading = true
				m.err = nil
				return m, loginCmd(m.authClient, m.emailInput, m.passwordInput)
			} else {
				m.err = fmt.Errorf("auth client not connected")
			}
		case "backspace":
			if m.focusedInput == 0 && len(m.emailInput) > 0 {
				m.emailInput = m.emailInput[:len(m.emailInput)-1]
			} else if m.focusedInput == 1 && len(m.passwordInput) > 0 {
				m.passwordInput = m.passwordInput[:len(m.passwordInput)-1]
			}
		case "ctrl+l":
			m.emailInput = ""
			m.passwordInput = ""
			m.err = nil
		default:
			if len(msg.String()) == 1 {
				if m.focusedInput == 0 {
					m.emailInput += msg.String()
				} else {
					m.passwordInput += msg.String()
				}
			}
		}
	}
	return m, nil
}

func (m *LoginModel) View() string {
	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true).
		Render("🔐 LOGIN")

	subtitle := lipgloss.NewStyle().
		Foreground(Muted).
		Render("Welcome back! Please sign in to continue.")

	b.WriteString(lipgloss.NewStyle().
		Width(80).
		Align(lipgloss.Center).
		MarginTop(2).
		Render(title))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Width(80).
		Align(lipgloss.Center).
		MarginBottom(3).
		Render(subtitle))
	b.WriteString("\n\n")

	// Email input
	emailLabel := LabelStyle.Width(15).Render("Email:")
	var emailInputStyle lipgloss.Style
	if m.focusedInput == 0 {
		emailInputStyle = FocusedInputStyle
	} else {
		emailInputStyle = InputStyle
	}
	emailValue := emailInputStyle.Width(50).Render(m.emailInput)
	emailField := lipgloss.JoinHorizontal(lipgloss.Left, emailLabel, emailValue)
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(emailField))
	b.WriteString("\n\n")

	// Password input
	passwordLabel := LabelStyle.Width(15).Render("Password:")
	var passwordInputStyle lipgloss.Style
	if m.focusedInput == 1 {
		passwordInputStyle = FocusedInputStyle
	} else {
		passwordInputStyle = InputStyle
	}
	// Mask password
	maskedPassword := strings.Repeat("•", len(m.passwordInput))
	passwordValue := passwordInputStyle.Width(50).Render(maskedPassword)
	passwordField := lipgloss.JoinHorizontal(lipgloss.Left, passwordLabel, passwordValue)
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(passwordField))
	b.WriteString("\n\n")

	// Status messages
	if m.loading {
		loading := InfoStyle.Render("🔄 Logging in...")
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(loading))
		b.WriteString("\n")
	}

	if m.err != nil {
		errMsg := ErrorStyle.Render("❌ " + m.err.Error())
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(errMsg))
		b.WriteString("\n")
	}

	// Help
	b.WriteString("\n")
	help := InfoStyle.Render("tab switch  •  enter login  •  ctrl+l clear  •  ctrl+s signup  •  q quit")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(help))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Primary).
		Padding(2, 4).
		Width(76).
		Render(b.String())
}
