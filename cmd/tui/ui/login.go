package ui

import (
	"fmt"
	"strings"

	"github.com/Varun5711/shorternit/cmd/tui/client"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// loginSuccessMsg is dispatched when the gRPC Login RPC succeeds. The parent
// Model intercepts it to update global auth state and persist the session.
type loginSuccessMsg struct {
	token  string
	userID string
	email  string
	name   string
}

// loginErrorMsg carries a login failure back to the LoginModel so it can
// display the error inline without crashing.
type loginErrorMsg struct {
	err error
}

// LoginModel manages the login form state: two text inputs (email, password),
// a focus tracker, loading flag, and any error from the last attempt.
// Input is handled character-by-character because Bubble Tea does not ship
// a built-in text-input component with password masking that fits the
// custom styling used here.
type LoginModel struct {
	emailInput    string
	passwordInput string
	focusedInput  int // 0 = email, 1 = password
	loading       bool
	err           error
	authClient    *client.AuthClient
}

// NewLoginModel creates a LoginModel with the email field focused.
func NewLoginModel() *LoginModel {
	return &LoginModel{
		focusedInput: 0,
	}
}

// SetAuthClient wires the gRPC auth client into the model after construction.
func (m *LoginModel) SetAuthClient(c *client.AuthClient) {
	m.authClient = c
}

// Init satisfies the tea.Model interface; no startup command is needed.
func (m *LoginModel) Init() tea.Cmd {
	return nil
}

// loginCmd returns a Bubble Tea Cmd that performs the gRPC login call in a
// background goroutine. On completion it dispatches either loginSuccessMsg
// or loginErrorMsg back into the Update loop.
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

// Update handles login form interaction: Tab to switch fields, Enter to
// submit, Backspace to delete, and ctrl+l to clear. While loading, all
// key input is ignored to prevent double-submission.
func (m *LoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loginSuccessMsg:
		m.loading = false
		m.err = nil

		return m, func() tea.Msg { return msg }

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

// View renders the login form inside a rounded border with email and
// password fields, a loading indicator, inline error display, and a
// help bar showing available keyboard shortcuts.
func (m *LoginModel) View() string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true).
		Render("🔐 LOGIN")

	subtitle := lipgloss.NewStyle().
		Foreground(Muted).
		Render("Welcome back! Please sign in to continue.")

	b.WriteString(lipgloss.NewStyle().
		Width(120).
		Align(lipgloss.Center).
		MarginTop(2).
		Render(title))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().
		Width(120).
		Align(lipgloss.Center).
		MarginBottom(3).
		Render(subtitle))
	b.WriteString("\n\n")

	emailLabel := LabelStyle.Width(15).Render("Email:")
	var emailInputStyle lipgloss.Style
	if m.focusedInput == 0 {
		emailInputStyle = FocusedInputStyle
	} else {
		emailInputStyle = InputStyle
	}
	emailValue := emailInputStyle.Width(70).Render(m.emailInput)
	emailField := lipgloss.JoinHorizontal(lipgloss.Left, emailLabel, emailValue)
	b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(emailField))
	b.WriteString("\n\n")

	passwordLabel := LabelStyle.Width(15).Render("Password:")
	var passwordInputStyle lipgloss.Style
	if m.focusedInput == 1 {
		passwordInputStyle = FocusedInputStyle
	} else {
		passwordInputStyle = InputStyle
	}

	maskedPassword := strings.Repeat("•", len(m.passwordInput))
	passwordValue := passwordInputStyle.Width(70).Render(maskedPassword)
	passwordField := lipgloss.JoinHorizontal(lipgloss.Left, passwordLabel, passwordValue)
	b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(passwordField))
	b.WriteString("\n\n")

	if m.loading {
		loading := InfoStyle.Render("🔄 Logging in...")
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(loading))
		b.WriteString("\n")
	}

	if m.err != nil {
		errMsg := ErrorStyle.Render("❌ " + m.err.Error())
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(errMsg))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	help := InfoStyle.Render("tab switch  •  enter login  •  ctrl+l clear  •  ctrl+s signup  •  q quit")
	b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(help))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Primary).
		Padding(2, 4).
		Width(116).
		Render(b.String())
}
