package tui

import (
	"llm_tui/internal/db"
	"llm_tui/internal/tui/views"

	tea "github.com/charmbracelet/bubbletea"
)

type Screen int

const (
	ScreenManager Screen = iota
	ScreenProbe
	ScreenTester
)

type AppModel struct {
	DB           *db.DB
	Screen       Screen
	ManagerView  views.ManagerModel
	ProbeView    views.ProbeModel
	TesterView   views.TesterModel
	Width        int
	Height       int
}

func NewAppModel(database *db.DB) AppModel {
	mgr := views.NewManagerModel(database)
	return AppModel{
		DB:          database,
		Screen:      ScreenManager,
		ManagerView: mgr,
	}
}

func (m AppModel) Init() tea.Cmd {
	return nil
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.ManagerView.Width = msg.Width
		m.ManagerView.Height = msg.Height
		m.ProbeView.Width = msg.Width
		m.ProbeView.Height = msg.Height
		m.TesterView.Width = msg.Width
		m.TesterView.Height = msg.Height

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	switch m.Screen {
	case ScreenManager:
		var action string
		m.ManagerView, cmd, action = m.ManagerView.Update(msg)
		switch action {
		case "probe_new":
			m.ProbeView = views.NewProbeModel(m.DB)
			m.Screen = ScreenProbe
			return m, m.ProbeView.Init()
		case "open_tester":
			rec := m.ManagerView.SelectedRecord()
			if rec != nil {
				m.TesterView = views.NewTesterModel(m.DB, *rec)
				m.Screen = ScreenTester
				return m, m.TesterView.Init()
			}
		}

	case ScreenProbe:
		var action string
		m.ProbeView, cmd, action = m.ProbeView.Update(msg)
		switch action {
		case "back_to_manager":
			m.Screen = ScreenManager
			m.ManagerView.RefreshRecords()
			return m, nil
		case "record_created":
			m.Screen = ScreenManager
			m.ManagerView.RefreshRecords()
			m.ManagerView.StatusMsg = "Provider record created and saved to SQLite!"
			m.ManagerView.IsError = false
			return m, nil
		}

	case ScreenTester:
		var action string
		m.TesterView, cmd, action = m.TesterView.Update(msg)
		switch action {
		case "back_to_manager":
			m.Screen = ScreenManager
			m.ManagerView.RefreshRecords()
			return m, nil
		}
	}

	return m, cmd
}

func (m AppModel) View() string {
	switch m.Screen {
	case ScreenProbe:
		return m.ProbeView.View()
	case ScreenTester:
		return m.TesterView.View()
	default:
		return m.ManagerView.View()
	}
}
