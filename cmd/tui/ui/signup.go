package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Varun5711/shorternit/cmd/tui/client"
)

type signupSuccessMsg struct {
	token  string
	userID string
	email  string
	name   string
}

type signupErrorMsg struct {
	err error
}

type SignupModel struct {
	nameInput     string
	emailInput    string
	passwordInput string
	focusedInput  int
	loading       bool
	err           error
	authClient    *client.AuthClient
}

func NewSignupModel() *SignupModel {
	return &SignupModel{
		focusedInput: 0,
	}
}

func (m *SignupModel) SetAuthClient(c *client.AuthClient) {
	m.authClient = c
}

func (m *SignupModel) Init() tea.Cmd {
	return nil
}

func signupCmd(c *client.AuthClient, email, password, name string) tea.Cmd {
	return func() tea.Msg {
		resp, err := c.Register(email, password, name)
		if err != nil {
			return signupErrorMsg{err: err}
		}

		return signupSuccessMsg{
			token:  resp.Token,
			userID: resp.UserId,
			email:  resp.Email,
			name:   resp.Name,
		}
	}
}

func (m *SignupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case signupSuccessMsg:
		m.loading = false
		m.err = nil
		return m, nil

	case signupErrorMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		switch msg.String() {
		case "tab", "shift+tab":
			m.focusedInput = (m.focusedInput + 1) % 3
		case "enter":
			if m.nameInput == "" {
				m.err = fmt.Errorf("name cannot be empty")
				return m, nil
			}
			if m.emailInput == "" {
				m.err = fmt.Errorf("email cannot be empty")
				return m, nil
			}
			if m.passwordInput == "" {
				m.err = fmt.Errorf("password cannot be empty")
				return m, nil
			}
			if len(m.passwordInput) < 8 {
				m.err = fmt.Errorf("password must be at least 8 characters")
				return m, nil
			}

			if m.authClient != nil {
				m.loading = true
				m.err = nil
				return m, signupCmd(m.authClient, m.emailInput, m.passwordInput, m.nameInput)
			} else {
				m.err = fmt.Errorf("auth client not connected")
			}
		case "backspace":
			if m.focusedInput == 0 && len(m.nameInput) > 0 {
				m.nameInput = m.nameInput[:len(m.nameInput)-1]
			} else if m.focusedInput == 1 && len(m.emailInput) > 0 {
				m.emailInput = m.emailInput[:len(m.emailInput)-1]
			} else if m.focusedInput == 2 && len(m.passwordInput) > 0 {
				m.passwordInput = m.passwordInput[:len(m.passwordInput)-1]
			}
		case "ctrl+l":
			m.nameInput = ""
			m.emailInput = ""
			m.passwordInput = ""
			m.err = nil
		default:
			if len(msg.String()) == 1 {
				if m.focusedInput == 0 {
					m.nameInput += msg.String()
				} else if m.focusedInput == 1 {
					m.emailInput += msg.String()
				} else {
					m.passwordInput += msg.String()
				}
			}
		}
	}
	return m, nil
}

func (m *SignupModel) View() string {
	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(Success).
		Bold(true).
		Render("✨ SIGN UP")

	subtitle := lipgloss.NewStyle().
		Foreground(Muted).
		Render("Create a new account to get started.")

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

	// Name input
	nameLabel := LabelStyle.Width(15).Render("Name:")
	var nameInputStyle lipgloss.Style
	if m.focusedInput == 0 {
		nameInputStyle = FocusedInputStyle
	} else {
		nameInputStyle = InputStyle
	}
	nameValue := nameInputStyle.Width(50).Render(m.nameInput)
	nameField := lipgloss.JoinHorizontal(lipgloss.Left, nameLabel, nameValue)
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(nameField))
	b.WriteString("\n\n")

	// Email input
	emailLabel := LabelStyle.Width(15).Render("Email:")
	var emailInputStyle lipgloss.Style
	if m.focusedInput == 1 {
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
	if m.focusedInput == 2 {
		passwordInputStyle = FocusedInputStyle
	} else {
		passwordInputStyle = InputStyle
	}
	// Mask password
	maskedPassword := strings.Repeat("•", len(m.passwordInput))
	passwordValue := passwordInputStyle.Width(50).Render(maskedPassword)
	passwordField := lipgloss.JoinHorizontal(lipgloss.Left, passwordLabel, passwordValue)
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(passwordField))
	b.WriteString("\n")

	passHint := InfoStyle.Render("(min 8 characters)")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(passHint))
	b.WriteString("\n\n")

	// Status messages
	if m.loading {
		loading := InfoStyle.Render("🔄 Creating account...")
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
	help := InfoStyle.Render("tab switch  •  enter signup  •  ctrl+l clear  •  ctrl+l login  •  q quit")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(help))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Success).
		Padding(2, 4).
		Width(76).
		Render(b.String())
}
