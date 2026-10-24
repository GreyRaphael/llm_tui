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
	Screen      Screen
	ManagerView views.ManagerModel
	ProbeView   views.ProbeModel
	TesterView  views.TesterModel
	Width       int
	Height      int
}

func NewAppModel(database *db.DB, version string) AppModel {
	if version == "" {
		version = "v1.0.0"
	}
	return AppModel{
		DB:          database,
		Screen:      ScreenManager,
		ManagerView: views.NewManagerModel(database, version),
	}
}

func (m AppModel) Init() tea.Cmd {
	return nil
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
