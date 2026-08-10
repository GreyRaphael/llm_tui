package views

import (
	"testing"

	"llm_tui/internal/api"
	"llm_tui/internal/db"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "******"},
		{"sk-1234", "******"},
		{"sk-12345", "******"},             // exactly 8 chars
		{"sk-123456", "sk-1...3456"},       // 9 chars: first 4 + ... + last 4
		{"sk-abcdefghijklmnop", "sk-a...mnop"},
	}

	for _, tt := range tests {
		got := maskAPIKey(tt.input)
		if got != tt.expected {
			t.Errorf("maskAPIKey(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestManagerModel_Navigation(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	// Create test records
	for _, name := range []string{"Provider A", "Provider B", "Provider C"} {
		_ = database.CreateRecord(&db.ProviderRecord{
			Name:    name,
			BaseURL: "https://api.example.com",
			APIKey:  "sk-test",
			APIType: "openai_chat",
			Model:   "gpt-4o",
		})
	}

	m := NewManagerModel(database)

	if len(m.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(m.Records))
	}
	if m.Cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", m.Cursor)
	}

	// Navigate down
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.Cursor != 1 {
		t.Errorf("expected cursor at 1 after 'j', got %d", m.Cursor)
	}

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.Cursor != 2 {
		t.Errorf("expected cursor at 2 after second 'j', got %d", m.Cursor)
	}

	// Don't go past end
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.Cursor != 2 {
		t.Errorf("expected cursor to stay at 2, got %d", m.Cursor)
	}

	// Navigate up
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.Cursor != 1 {
		t.Errorf("expected cursor at 1 after 'k', got %d", m.Cursor)
	}
}

func TestManagerModel_DeleteConfirmation(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	_ = database.CreateRecord(&db.ProviderRecord{
		Name:    "Test Provider",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-test",
		APIType: "openai_chat",
		Model:   "gpt-4o",
	})

	m := NewManagerModel(database)
	if len(m.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(m.Records))
	}

	// First 'd' press: should ask for confirmation, NOT delete
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !m.ConfirmDelete {
		t.Fatal("expected ConfirmDelete to be true after first 'd'")
	}
	if len(m.Records) != 1 {
		t.Fatal("record should NOT be deleted after first 'd'")
	}

	// Press another key: should cancel confirmation
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if m.ConfirmDelete {
		t.Fatal("expected ConfirmDelete to be false after pressing 'k'")
	}
	if len(m.Records) != 1 {
		t.Fatal("record should still exist after canceling")
	}

	// First 'd' again
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !m.ConfirmDelete {
		t.Fatal("expected ConfirmDelete to be true")
	}

	// Second 'd': should actually delete
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if m.ConfirmDelete {
		t.Fatal("expected ConfirmDelete to be false after deletion")
	}
	if len(m.Records) != 0 {
		t.Fatalf("expected 0 records after deletion, got %d", len(m.Records))
	}
}

func TestManagerModel_Actions(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	_ = database.CreateRecord(&db.ProviderRecord{
		Name:    "Test",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-test",
		APIType: "openai_chat",
		Model:   "gpt-4o",
	})

	m := NewManagerModel(database)

	// 'n' should return "probe_new" action
	_, _, action := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if action != "probe_new" {
		t.Errorf("expected action 'probe_new', got %q", action)
	}

	// 'enter' should return "open_tester" action
	_, _, action = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != "open_tester" {
		t.Errorf("expected action 'open_tester', got %q", action)
	}

	// 'q' should return "quit" action
	_, _, action = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if action != "quit" {
		t.Errorf("expected action 'quit', got %q", action)
	}
}

func TestManagerModel_SelectedRecord(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewManagerModel(database)

	// Empty records: should return nil
	if rec := m.SelectedRecord(); rec != nil {
		t.Error("expected nil SelectedRecord when no records exist")
	}

	_ = database.CreateRecord(&db.ProviderRecord{
		Name:    "Test",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-test",
		APIType: "openai_chat",
		Model:   "gpt-4o",
	})
	m.RefreshRecords()

	rec := m.SelectedRecord()
	if rec == nil {
		t.Fatal("expected non-nil SelectedRecord")
	}
	if rec.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", rec.Name)
	}
}

func TestTesterModel_ReasoningEffortShortcuts(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	rec := db.ProviderRecord{
		Name:    "Test",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-test",
		APIType: api.APITypeOpenAIChat,
		Model:   "gpt-4o",
	}
	m := NewTesterModel(database, rec)
	m.Resize(120, 40)

	if m.ReasoningEffort != db.ReasoningEffortNone {
		t.Fatalf("expected initial reasoning effort 'none', got %q", m.ReasoningEffort)
	}

	// Verify Resize sets dimensions
	if m.Width != 120 || m.Height != 40 {
		t.Errorf("expected dimensions 120x40, got %dx%d", m.Width, m.Height)
	}
}

func TestProbeModel_Resize(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewProbeModel(database)
	m.Resize(120, 40)

	if m.Width != 120 || m.Height != 40 {
		t.Errorf("expected dimensions 120x40, got %dx%d", m.Width, m.Height)
	}

	expectedInputWidth := 110 // 120 - 10
	if m.BaseURLInput.Width != expectedInputWidth {
		t.Errorf("expected input width %d, got %d", expectedInputWidth, m.BaseURLInput.Width)
	}

	// Small window should clamp to minimum 50
	m.Resize(40, 20)
	if m.BaseURLInput.Width != 50 {
		t.Errorf("expected input width clamped to 50, got %d", m.BaseURLInput.Width)
	}
}
