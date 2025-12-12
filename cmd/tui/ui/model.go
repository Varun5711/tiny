package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/Varun5711/shorternit/cmd/tui/client"
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

	return Model{
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
		m.currentView = MenuView
		return m, nil

	case signupSuccessMsg:
		m.isAuthenticated = true
		m.token = msg.token
		m.userID = msg.userID
		m.userName = msg.name
		m.userEmail = msg.email
		m.client.SetAuth(msg.token, msg.userID)
		m.analytics.SetToken(msg.token)
		m.currentView = MenuView
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "q":
			if m.currentView == MenuView || m.currentView == LoginView || m.currentView == SignupView {
				return m, tea.Quit
			}
			// Go back to menu
			m.currentView = MenuView
			return m, nil

		case "ctrl+s":
			// Toggle between login and signup
			if m.currentView == LoginView {
				m.currentView = SignupView
				return m, nil
			} else if m.currentView == SignupView {
				m.currentView = LoginView
				return m, nil
			}

		case "ctrl+m":
			// Go to menu (only when authenticated)
			if m.isAuthenticated {
				m.currentView = MenuView
				return m, nil
			}
		}
	}

	// Route to appropriate view
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
			// Map menu selection to view and trigger data load
			switch m.menu.selected {
			case 0:
				m.currentView = CreateView
			case 1:
				m.currentView = ListView
				// Reset list model to trigger auto-load
				m.list.loaded = false
			case 2:
				m.currentView = AnalyticsView
				// Reset analytics model to trigger auto-load
				m.analytics.loaded = false
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
			Width(80).
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

	// Combine status bar and content
	if statusBar != "" {
		return lipgloss.JoinVertical(lipgloss.Left, statusBar, "\n", mainContent)
	}
	return mainContent
}
