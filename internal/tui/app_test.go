package tui

import (
	"testing"

	"llm_tui/internal/api"
	"llm_tui/internal/db"

	tea "github.com/charmbracelet/bubbletea"
)

func updateApp(t *testing.T, app AppModel, msg tea.Msg) (AppModel, tea.Cmd) {
	t.Helper()
	updated, cmd := app.Update(msg)
	next, ok := updated.(AppModel)
	if !ok {
		t.Fatalf("unexpected model type %T", updated)
	}
	return next, cmd
}

func TestAppModelNavigationAndQuit(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()
	rec := &db.ProviderRecord{Name: "test", BaseURL: "http://localhost", APIType: api.APITypeOpenAIChat, Model: "gpt-4o"}
	if err := database.CreateRecord(rec); err != nil {
		t.Fatalf("CreateRecord failed: %v", err)
	}

	app := NewAppModel(database, "v-test")
	app, _ = updateApp(t, app, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if app.Screen != ScreenProbe {
		t.Fatalf("expected probe screen, got %v", app.Screen)
	}
	app, _ = updateApp(t, app, tea.KeyMsg{Type: tea.KeyEsc})
	if app.Screen != ScreenManager {
		t.Fatalf("expected manager after escape, got %v", app.Screen)
	}

	app, cmd := updateApp(t, app, tea.KeyMsg{Type: tea.KeyEnter})
	if app.Screen != ScreenTester || cmd == nil {
		t.Fatal("expected initialized tester screen")
	}
	app, _ = updateApp(t, app, tea.KeyMsg{Type: tea.KeyEsc})
	if app.Screen != ScreenManager {
		t.Fatalf("expected manager after tester escape, got %v", app.Screen)
	}

	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestAppModelDefaultVersion(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()
	app := NewAppModel(database, "")
	if app.ManagerView.Version != "v1.0.0" {
		t.Fatalf("unexpected default version %q", app.ManagerView.Version)
	}
}
