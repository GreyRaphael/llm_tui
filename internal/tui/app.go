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
	DB          *db.DB
	Version     string
	Screen      Screen
	ManagerView views.ManagerModel
	ProbeView   views.ProbeModel
	TesterView  views.TesterModel
	Width       int
	Height      int
}

func NewAppModel(database *db.DB, version ...string) AppModel {
	ver := "v1.0.0"
	if len(version) > 0 && version[0] != "" {
		ver = version[0]
	}
	mgr := views.NewManagerModel(database, ver)
	return AppModel{
		DB:          database,
		Version:     ver,
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
		switch m.Screen {
		case ScreenProbe:
			m.ProbeView.Resize(msg.Width, msg.Height)
		case ScreenTester:
			m.TesterView.Resize(msg.Width, msg.Height)
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.Screen == ScreenManager && msg.String() == "q" {
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
			if m.Width > 0 && m.Height > 0 {
				m.ProbeView.Resize(m.Width, m.Height)
			}
			m.Screen = ScreenProbe
			return m, m.ProbeView.Init()
		case "open_tester":
			rec := m.ManagerView.SelectedRecord()
			if rec != nil {
				m.TesterView = views.NewTesterModel(m.DB, *rec)
				if m.Width > 0 && m.Height > 0 {
					m.TesterView.Resize(m.Width, m.Height)
				}
				m.Screen = ScreenTester
				return m, m.TesterView.Init()
			}
		case "quit":
			return m, tea.Quit
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
