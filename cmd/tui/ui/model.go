// Package ui implements the Bubble Tea model and views for the Tiny TUI
// client. It follows the Elm Architecture: a single top-level Model holds
// the application state (current view, sub-models for each screen, auth
// credentials), and the Update method routes messages to the active view's
// sub-model. View rendering is delegated to per-screen View() methods that
// use Lipgloss for styling.
//
// Navigation works as a simple state machine: the currentView field
// determines which sub-model receives updates and which View() is rendered.
// Global key bindings (ctrl+c to quit, q to go back, ctrl+s to toggle
// login/signup) are handled at the top level before delegation.
package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Varun5711/shorternit/cmd/tui/client"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// View enumerates the screens the TUI can display. The iota ordering
// matches the typical user flow: login -> signup -> menu -> actions.
type View int

const (
	LoginView View = iota
	SignupView
	MenuView
	CreateView
	ListView
	AnalyticsView
)

// SessionData is the JSON structure persisted to ~/.tiny_session.json.
// Storing the token locally lets the TUI skip the login screen on
// subsequent launches. The file is written with 0600 permissions to
// prevent other users on the same machine from reading the JWT.
type SessionData struct {
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
}

// getSessionPath returns the absolute path to the session file in the
// user's home directory.
func getSessionPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".tiny_session.json")
}

// saveSession writes authentication state to disk so the user does not
// need to log in again on the next TUI launch.
func saveSession(data SessionData) error {
	sessionPath := getSessionPath()
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath, jsonData, 0600)
}

// loadSession reads a previously saved session from disk. Returns an error
// if the file does not exist or is malformed, which the caller treats as
// "not logged in".
func loadSession() (*SessionData, error) {
	sessionPath := getSessionPath()
	jsonData, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, err
	}
	var data SessionData
	err = json.Unmarshal(jsonData, &data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

// clearSession deletes the persisted session file, effectively logging
// the user out across TUI restarts.
func clearSession() error {
	sessionPath := getSessionPath()
	return os.Remove(sessionPath)
}

// Model is the top-level Bubble Tea model. It owns every sub-model and
// routes messages to the currently active view. Auth state (token, userID,
// etc.) lives here rather than in individual views so that view transitions
// can propagate credentials without coupling sub-models to each other.
type Model struct {
	currentView View
	login       *LoginModel
	signup      *SignupModel
	menu        *MenuModel
	create      *CreateModel
	list        *ListModel
	analytics   *AnalyticsModel
	client      *client.Client
	authClient  *client.AuthClient
	width       int
	height      int
	err         error

	// Auth state
	isAuthenticated bool
	token           string
	userID          string
	userName        string
	userEmail       string
}

// NewModel initializes the top-level Model with all sub-models and wires
// the gRPC clients into the views that need them. If a valid session file
// exists on disk, the user is automatically logged in and dropped into the
// menu view, skipping the login screen entirely.
func NewModel(grpcClient *client.Client, authClient *client.AuthClient) Model {
	loginModel := NewLoginModel()
	loginModel.SetAuthClient(authClient)

	signupModel := NewSignupModel()
	signupModel.SetAuthClient(authClient)

	createModel := NewCreateModel()
	createModel.SetClient(grpcClient)

	listModel := NewListModel()
	listModel.SetClient(grpcClient)

	analyticsModel := NewAnalyticsModel()

	m := Model{
		currentView:     LoginView,
		login:           loginModel,
		signup:          signupModel,
		menu:            NewMenuModel(),
		create:          createModel,
		list:            listModel,
		analytics:       analyticsModel,
		client:          grpcClient,
		authClient:      authClient,
		isAuthenticated: false,
	}

	if session, err := loadSession(); err == nil {

		m.isAuthenticated = true
		m.token = session.Token
		m.userID = session.UserID
		m.userName = session.UserName
		m.userEmail = session.UserEmail
		m.client.SetAuth(session.Token, session.UserID)
		m.analytics.SetToken(session.Token)
		m.menu.SetUserName(session.UserName)
		m.currentView = MenuView
	}

	return m
}

// Init satisfies the tea.Model interface. No initial commands are needed
// because the first render is driven by the view state set in NewModel.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update is the central message router. It handles three categories:
//  1. Window resize events -- stored for responsive layout.
//  2. Auth success messages -- bubble up from login/signup sub-models to
//     update the top-level auth state and persist the session.
//  3. Key events -- global shortcuts (quit, back, toggle login/signup)
//     are intercepted before delegating to the active sub-model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case loginSuccessMsg:
		m.isAuthenticated = true
		m.token = msg.token
		m.userID = msg.userID
		m.userName = msg.name
		m.userEmail = msg.email
		m.client.SetAuth(msg.token, msg.userID)
		m.analytics.SetToken(msg.token)
		m.menu.SetUserName(msg.name)
		m.currentView = MenuView

		saveSession(SessionData{
			Token:     msg.token,
			UserID:    msg.userID,
			UserName:  msg.name,
			UserEmail: msg.email,
		})

		return m, nil

	case signupSuccessMsg:
		m.isAuthenticated = true
		m.token = msg.token
		m.userID = msg.userID
		m.userName = msg.name
		m.userEmail = msg.email
		m.client.SetAuth(msg.token, msg.userID)
		m.analytics.SetToken(msg.token)
		m.menu.SetUserName(msg.name)
		m.currentView = MenuView

		saveSession(SessionData{
			Token:     msg.token,
			UserID:    msg.userID,
			UserName:  msg.name,
			UserEmail: msg.email,
		})

		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "q":
			if m.currentView == MenuView || m.currentView == LoginView || m.currentView == SignupView {
				return m, tea.Quit
			}

			m.currentView = MenuView
			return m, nil

		case "ctrl+s":

			if m.currentView == LoginView {
				m.currentView = SignupView
				return m, nil
			} else if m.currentView == SignupView {
				m.currentView = LoginView
				return m, nil
			}

		case "ctrl+m":

			if m.isAuthenticated {
				m.currentView = MenuView
				return m, nil
			}
		}
	}

	switch m.currentView {
	case LoginView:
		updatedLogin, cmd := m.login.Update(msg)
		m.login = updatedLogin.(*LoginModel)
		return m, cmd

	case SignupView:
		updatedSignup, cmd := m.signup.Update(msg)
		m.signup = updatedSignup.(*SignupModel)
		return m, cmd

	case MenuView:
		updatedMenu, cmd := m.menu.Update(msg)
		m.menu = updatedMenu.(*MenuModel)
		if m.menu.selected != -1 {

			switch m.menu.selected {
			case 0:
				m.currentView = CreateView
			case 1:
				m.currentView = ListView

				m.list.loaded = false

				updatedList, listCmd := m.list.Update(nil)
				m.list = updatedList.(*ListModel)
				m.menu.selected = -1
				return m, listCmd
			case 2:
				m.currentView = AnalyticsView

				m.analytics.loaded = false

				updatedAnalytics, analyticsCmd := m.analytics.Update(nil)
				m.analytics = updatedAnalytics.(*AnalyticsModel)
				m.menu.selected = -1
				return m, analyticsCmd
			case 3:

				clearSession()
				m.isAuthenticated = false
				m.token = ""
				m.userID = ""
				m.userName = ""
				m.userEmail = ""
				m.currentView = LoginView
			}
			m.menu.selected = -1
		}
		return m, cmd

	case CreateView:
		updatedCreate, cmd := m.create.Update(msg)
		m.create = updatedCreate.(*CreateModel)
		return m, cmd

	case ListView:
		updatedList, cmd := m.list.Update(msg)
		m.list = updatedList.(*ListModel)
		return m, cmd

	case AnalyticsView:
		updatedAnalytics, cmd := m.analytics.Update(msg)
		m.analytics = updatedAnalytics.(*AnalyticsModel)
		return m, cmd
	}

	return m, nil
}

// View renders the active screen with an optional status bar showing the
// logged-in user's name and email. The status bar appears on all
// authenticated views but is hidden on login/signup to keep those screens
// clean.
func (m Model) View() string {
	// Status bar (shown when authenticated)
	var statusBar string
	if m.isAuthenticated && m.currentView != LoginView && m.currentView != SignupView {
		userInfo := lipgloss.NewStyle().
			Foreground(Success).
			Render("👤 " + m.userName)

		emailInfo := lipgloss.NewStyle().
			Foreground(Muted).
			Render(" (" + m.userEmail + ")")

		statusBar = lipgloss.NewStyle().
			Width(120).
			Align(lipgloss.Left).
			Background(BgDark).
			Padding(0, 2).
			Render(userInfo + emailInfo)
	}

	// Main content
	var mainContent string
	switch m.currentView {
	case LoginView:
		mainContent = m.login.View()
	case SignupView:
		mainContent = m.signup.View()
	case MenuView:
		mainContent = m.menu.View()
	case CreateView:
		mainContent = m.create.View()
	case ListView:
		mainContent = m.list.View()
	case AnalyticsView:
		mainContent = m.analytics.View()
	}

	if statusBar != "" {
		return lipgloss.JoinVertical(lipgloss.Left, statusBar, "\n", mainContent)
	}
	return mainContent
}
