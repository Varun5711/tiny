package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Varun5711/shorternit/cmd/tui/client"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type View int

const (
	LoginView View = iota
	SignupView
	MenuView
	CreateView
	ListView
	AnalyticsView
)

type SessionData struct {
	Token     string `json:"token"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	UserEmail string `json:"user_email"`
}

func getSessionPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".tiny_session.json")
}

func saveSession(data SessionData) error {
	sessionPath := getSessionPath()
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath, jsonData, 0600)
}

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

func clearSession() error {
	sessionPath := getSessionPath()
	return os.Remove(sessionPath)
}

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

func (m Model) Init() tea.Cmd {
	return nil
}

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
