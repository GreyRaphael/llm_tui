package tui

import (
	"llm_tui/internal/tui/views"

	tea "github.com/charmbracelet/bubbletea"
)

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			switch m.Screen {
			case ScreenProbe:
				m.ProbeView.CancelRequest()
			case ScreenTester:
				m.TesterView.CancelRequests()
			}
			return m, tea.Quit
		}
		if m.Screen == ScreenManager && msg.String() == "q" {
			return m, tea.Quit
		}
	}

	switch m.Screen {
	case ScreenProbe:
		return m.updateProbeScreen(msg)
	case ScreenTester:
		return m.updateTesterScreen(msg)
	default:
		return m.updateManagerScreen(msg)
	}
}

func (m *AppModel) resize(width, height int) {
	m.Width = width
	m.Height = height
	m.ManagerView.Resize(width, height)
	switch m.Screen {
	case ScreenProbe:
		m.ProbeView.Resize(width, height)
	case ScreenTester:
		m.TesterView.Resize(width, height)
	}
}

func (m AppModel) updateManagerScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd, action := m.ManagerView.Update(msg)
	m.ManagerView = updated
	switch action {
	case views.ActionProbeNew:
		m.ProbeView = views.NewProbeModel(m.DB)
		if m.Width > 0 && m.Height > 0 {
			m.ProbeView.Resize(m.Width, m.Height)
		}
		m.Screen = ScreenProbe
		return m, tea.Batch(cmd, m.ProbeView.Init())
	case views.ActionOpenTester:
		record := m.ManagerView.SelectedRecord()
		if record == nil {
			return m, cmd
		}
		m.TesterView = views.NewTesterModel(m.DB, *record)
		if m.Width > 0 && m.Height > 0 {
			m.TesterView.Resize(m.Width, m.Height)
		}
		m.Screen = ScreenTester
		return m, tea.Batch(cmd, m.TesterView.Init())
	default:
		return m, cmd
	}
}

func (m AppModel) updateProbeScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd, action := m.ProbeView.Update(msg)
	m.ProbeView = updated
	switch action {
	case views.ActionBackToManager:
		return m.showManager(""), cmd
	case views.ActionRecordCreated:
		return m.showManager("Provider record created and saved to SQLite!"), cmd
	default:
		return m, cmd
	}
}

func (m AppModel) updateTesterScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	updated, cmd, action := m.TesterView.Update(msg)
	m.TesterView = updated
	if action == views.ActionBackToManager {
		return m.showManager(""), cmd
	}
	return m, cmd
}

func (m AppModel) showManager(status string) AppModel {
	m.Screen = ScreenManager
	m.ManagerView.RefreshRecords()
	if status != "" {
		m.ManagerView.SetStatus(status, false)
	}
	if m.Width > 0 && m.Height > 0 {
		m.ManagerView.Resize(m.Width, m.Height)
	}
	return m
}
