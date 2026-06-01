package ui

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Varun5711/shorternit/cmd/tui/client"
	pb "github.com/Varun5711/shorternit/proto/url"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// createURLSuccessMsg is dispatched when the gRPC CreateURL or
// CreateCustomURL RPC succeeds. It carries the generated short URL.
type createURLSuccessMsg struct {
	shortURL string
}

// copySuccessMsg signals that the short URL was copied to the clipboard.
type copySuccessMsg struct{}

// copyErrorMsg carries a clipboard-copy failure.
type copyErrorMsg struct {
	err error
}

// createURLErrorMsg carries a URL creation failure.
type createURLErrorMsg struct {
	err error
}

// CreateModel manages the URL creation form: a long-URL input, an optional
// custom alias input, and post-creation state (the result URL, clipboard
// copy status). It handles both auto-generated and custom short codes by
// branching on whether the alias field is empty.
type CreateModel struct {
	urlInput     string
	aliasInput   string
	focusedInput int // 0 = URL input, 1 = alias input
	loading      bool
	result       string // the short URL returned after successful creation
	copied       bool   // whether the result has been copied to clipboard
	err          error
	client       *client.Client
}

// Init satisfies the tea.Model interface; no startup command is needed.
func (m *CreateModel) Init() tea.Cmd {
	return nil
}

// NewCreateModel creates a CreateModel with the URL field focused.
func NewCreateModel() *CreateModel {
	return &CreateModel{
		focusedInput: 0,
	}
}

// SetClient wires the gRPC URL client into the model.
func (m *CreateModel) SetClient(c *client.Client) {
	m.client = c
}

// validateURL performs client-side URL validation before sending to the
// server. It checks for a non-empty value, an HTTP(S) scheme, valid URL
// structure, and a non-empty host component. This gives immediate feedback
// in the TUI without a round-trip to the backend.
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

// validateAlias performs lightweight client-side alias validation. It mirrors
// the server-side rules (min 3 chars, max 50 chars, URL-safe characters) but
// does not check reserved words or profanity -- that is the server's job.
// An empty alias is valid because it means "use auto-generated code".
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

// createURLCmd returns a Bubble Tea Cmd that calls either CreateCustomURL
// (when an alias is provided) or CreateURL (for auto-generated codes) in a
// background goroutine. The default expiration is set to 3 days from now.
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

		var shortURL string
		switch r := resp.(type) {
		case *pb.CreateURLResponse:
			shortURL = r.ShortUrl
		case *pb.CreateCustomURLResponse:
			shortURL = r.ShortUrl
		}

		return createURLSuccessMsg{
			shortURL: shortURL,
		}
	}
}

// copyToClipboard copies text to the system clipboard using a platform-
// specific command (pbcopy on macOS, xclip on Linux, clip on Windows).
// The copy runs in a background goroutine and dispatches a success or
// error message on completion.
func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		case "linux":
			cmd = exec.Command("xclip", "-selection", "clipboard")
		case "windows":
			cmd = exec.Command("clip")
		default:
			return copyErrorMsg{err: fmt.Errorf("unsupported platform")}
		}

		in, err := cmd.StdinPipe()
		if err != nil {
			return copyErrorMsg{err: err}
		}

		if err := cmd.Start(); err != nil {
			return copyErrorMsg{err: err}
		}

		if _, err := in.Write([]byte(text)); err != nil {
			return copyErrorMsg{err: err}
		}

		if err := in.Close(); err != nil {
			return copyErrorMsg{err: err}
		}

		if err := cmd.Wait(); err != nil {
			return copyErrorMsg{err: err}
		}

		return copySuccessMsg{}
	}
}

// Update handles the create-URL form interaction: Tab to switch fields,
// Enter to validate and submit, Shift+C to copy the result, and ctrl+l
// to clear all fields and start over.
func (m *CreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case createURLSuccessMsg:
		m.loading = false
		m.result = msg.shortURL
		m.copied = false
		m.err = nil
		return m, nil

	case createURLErrorMsg:
		m.loading = false
		m.err = msg.err
		m.result = ""
		m.copied = false
		return m, nil

	case copySuccessMsg:
		m.copied = true
		return m, nil

	case copyErrorMsg:
		m.err = msg.err
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
		case "C":

			if m.result != "" {
				return m, copyToClipboard(m.result)
			}
		case "ctrl+l":
			m.urlInput = ""
			m.aliasInput = ""
			m.result = ""
			m.copied = false
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

// View renders the URL creation form with the long-URL and optional alias
// inputs, a loading spinner, the resulting short URL (with copy hint), and
// any validation or server errors.
func (m *CreateModel) View() string {
	var b strings.Builder

	icon := lipgloss.NewStyle().Foreground(Accent).Render("🔗")
	header := icon + " " + TitleStyle.Render("CREATE SHORT URL") + " " + icon
	b.WriteString(lipgloss.NewStyle().
		Width(120).
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
	urlValue := urlInputStyle.Width(70).Render(m.urlInput)
	urlField := lipgloss.JoinHorizontal(lipgloss.Left, urlLabel, urlValue)
	b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(urlField))
	b.WriteString("\n\n")

	aliasLabel := LabelStyle.Render("Custom Alias:")
	aliasHint := InfoStyle.Render(" (optional)")
	var aliasInputStyle lipgloss.Style
	if m.focusedInput == 1 {
		aliasInputStyle = FocusedInputStyle
	} else {
		aliasInputStyle = InputStyle
	}
	aliasValue := aliasInputStyle.Width(70).Render(m.aliasInput)
	aliasField := lipgloss.JoinHorizontal(lipgloss.Left, aliasLabel, aliasValue, aliasHint)
	b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(aliasField))
	b.WriteString("\n\n")

	if m.loading {
		loading := InfoStyle.Render("Creating short URL...")
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(loading))
		b.WriteString("\n")
	}

	if m.result != "" {

		label := SuccessStyle.Render("✓ Short URL created:")
		urlStyle := lipgloss.NewStyle().
			Foreground(Primary).
			Underline(true).
			Bold(true)
		styledURL := urlStyle.Render("👉 " + m.result)
		result := label + "\n" + styledURL
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(result))
		b.WriteString("\n\n")

		if m.copied {
			copied := InfoStyle.Render("✓ Copied to clipboard!")
			b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(copied))
			b.WriteString("\n")
		} else {
			copyHint := InfoStyle.Render("Shift+C to copy  •  cmd+click to open")
			b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(copyHint))
			b.WriteString("\n")
		}
	}

	if m.err != nil {
		errMsg := ErrorStyle.Render("Error: " + m.err.Error())
		b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(errMsg))
		b.WriteString("\n")
	}

	help := InfoStyle.Render("tab switch  •  enter submit  •  ctrl+l clear  •  q back")
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Width(120).Align(lipgloss.Center).Render(help))

	return BoxStyle.Width(116).Render(b.String())
}
