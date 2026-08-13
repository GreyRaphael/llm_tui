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

func TestProbeModel_AllowsEmptyAPIKey(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewProbeModel(database)
	if m.Step != StepInputCredentials {
		t.Fatalf("expected start at StepInputCredentials, got %v", m.Step)
	}

	// Enter a Base URL, leave API Key empty, then submit.
	m.BaseURLInput.SetValue("https://api.example.com")
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.StatusMsg == "Please enter a Base URL" {
		t.Fatalf("empty API key should not block Base URL-only submission")
	}
	// Enter with empty API key should proceed to fetching models.
	if m.Step != StepFetchingModels {
		t.Fatalf("expected StepFetchingModels after submitting with empty API key, got %v (msg=%q)", m.Step, m.StatusMsg)
	}
}

func TestProbeModel_RequiresManualModelWhenFetchFails(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewProbeModel(database)
	m.Step = StepFetchingModels

	// No models discovered: input must not be pre-filled with a default model.
	m, _, _ = m.Update(modelsFetchedMsg{models: nil, err: nil})
	if m.Step != StepSelectModel {
		t.Fatalf("expected StepSelectModel, got %v", m.Step)
	}
	if got := m.ModelInput.Value(); got != "" {
		t.Fatalf("expected empty model input on fetch failure, got %q", got)
	}
	if m.SelectedModel != "" {
		t.Fatalf("expected empty SelectedModel on fetch failure, got %q", m.SelectedModel)
	}

	// Pressing Enter with no model must be rejected.
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Step != StepSelectModel {
		t.Fatalf("expected still StepSelectModel when no model entered, got %v", m.Step)
	}
	if !m.IsError {
		t.Fatal("expected error when submitting without a model")
	}

	// Entering a model id manually should proceed to probing.
	m.ModelInput.SetValue("deepseek-v4-flash")
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Step != StepProbing {
		t.Fatalf("expected StepProbing after manual model entry, got %v", m.Step)
	}
	if m.SelectedModel != "deepseek-v4-flash" {
		t.Fatalf("expected SelectedModel to be manual entry, got %q", m.SelectedModel)
	}
}

func TestProbeModel_APITypeSwitchKeepsAliasInSync(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewProbeModel(database)
	m.Step = StepSelectAPITypeAndName
	m.SelectedModel = "deepseek-v4-flash"
	m.ProbeResult = &api.ProbeResult{
		BaseURL:           "https://api.example.com",
		SupportedAPITypes: []string{api.APITypeOpenAIResponses, api.APITypeAnthropic},
	}
	m.SelectedAPIType = api.APITypeOpenAIResponses
	m.APITypeCursor = 0
	m.NameInput.SetValue("deepseek-v4-flash")

	// Move down to the second API type; the auto-generated alias must follow.
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.SelectedAPIType != api.APITypeAnthropic {
		t.Fatalf("expected selected API type anthropic, got %q", m.SelectedAPIType)
	}
	if got := m.NameInput.Value(); got != "deepseek-v4-flash" {
		t.Fatalf("expected alias to follow selected API type, got %q", got)
	}

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	records, err := database.ListRecords()
	if err != nil {
		t.Fatalf("ListRecords failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 saved record, got %d", len(records))
	}
	if records[0].APIType != api.APITypeAnthropic {
		t.Errorf("expected saved APIType anthropic, got %q", records[0].APIType)
	}
	if records[0].Name != "deepseek-v4-flash" {
		t.Errorf("expected alias to match saved APIType, got %q", records[0].Name)
	}
}

func TestProbeModel_APITypeSwitchUpdatesLegacyAlias(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewProbeModel(database)
	m.Step = StepSelectAPITypeAndName
	m.SelectedModel = "deepseek-v4-flash"
	m.ProbeResult = &api.ProbeResult{
		BaseURL:           "https://api.example.com",
		SupportedAPITypes: []string{api.APITypeOpenAIResponses, api.APITypeAnthropic},
	}
	m.SelectedAPIType = api.APITypeOpenAIResponses
	m.APITypeCursor = 0
	// Simulate a record that was created before the suffix was dropped.
	m.NameInput.SetValue("deepseek-v4-flash (openai_responses)")

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.NameInput.Value(); got != "deepseek-v4-flash" {
		t.Fatalf("expected legacy alias revisited, got %q", got)
	}
}

func TestProbeModel_APITypeSwitchPreservesCustomName(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewProbeModel(database)
	m.Step = StepSelectAPITypeAndName
	m.SelectedModel = "deepseek-v4-flash"
	m.ProbeResult = &api.ProbeResult{
		BaseURL:           "https://api.example.com",
		SupportedAPITypes: []string{api.APITypeOpenAIResponses, api.APITypeAnthropic},
	}
	m.SelectedAPIType = api.APITypeOpenAIResponses
	m.APITypeCursor = 0
	m.NameInput.SetValue("My DeepSeek Instance")

	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if got := m.NameInput.Value(); got != "My DeepSeek Instance" {
		t.Fatalf("custom name should not be overwritten, got %q", got)
	}
}

func TestTesterModel_StreamMsgUpdate(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	rec := db.ProviderRecord{
		Name:    "Test DeepSeek",
		BaseURL: "https://api.deepseek.com",
		APIKey:  "sk-test-key",
		APIType: api.APITypeOpenAIChat,
		Model:   "deepseek-reasoner",
	}
	m := NewTesterModel(database, rec)
	m.Resize(100, 30)

	// Send reasoning chunk msg
	m, _, _ = m.Update(api.StreamChunkMsg{
		StatusCode:     200,
		ReasoningDelta: "Analyzing request... ",
	})

	// Send content chunk msg
	m, _, _ = m.Update(api.StreamChunkMsg{
		ContentDelta: "Hello World!",
		PromptTokens: 10,
		CompletionTokens: 20,
		TotalTokens: 30,
	})

	if m.ReasoningText != "Analyzing request... " {
		t.Errorf("unexpected reasoning content: %q", m.ReasoningText)
	}
	if m.ContentText != "Hello World!" {
		t.Errorf("unexpected content content: %q", m.ContentText)
	}

	// Send stream finish msg
	m, _, _ = m.Update(api.StreamChunkMsg{
		Done: true,
	})

	if m.IsExecuting {
		t.Errorf("expected IsExecuting to be false after stream done")
	}
	if m.LastResult == nil {
		t.Fatalf("expected non-nil LastResult after stream done")
	}
	if m.LastResult.PromptTokens != 10 || m.LastResult.CompletionTokens != 20 || m.LastResult.TotalTokens != 30 {
		t.Errorf("unexpected token usage in LastResult: %+v", m.LastResult)
	}
}

func TestTesterModel_StreamModeDetection(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	rec := db.ProviderRecord{
		Name:    "Test Provider",
		BaseURL: "https://api.openai.com",
		APIKey:  "sk-test",
		APIType: api.APITypeOpenAIChat,
		Model:   "gpt-4o",
	}

	m := NewTesterModel(database, rec)

	// Test payload with stream: false
	m.Textarea.SetValue(`{"model":"gpt-4o","stream":false,"messages":[{"role":"user","content":"hi"}]}`)
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.IsStreamMode {
		t.Errorf("expected IsStreamMode to be false for stream: false payload")
	}

	// Test payload with stream: true
	m.Textarea.SetValue(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.IsStreamMode {
		t.Errorf("expected IsStreamMode to be true for stream: true payload")
	}
}

func TestTesterModel_ViewportWordWrap(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	rec := db.ProviderRecord{
		Name:    "Test Provider",
		BaseURL: "https://api.openai.com",
		APIKey:  "sk-test",
		APIType: api.APITypeOpenAIChat,
		Model:   "gpt-4o",
	}

	m := NewTesterModel(database, rec)
	m.Resize(60, 30) // viewport width will be around 24

	longLine := "This is a very long line of text that exceeds the viewport width and must be automatically wrapped into multiple lines by Lipgloss reflow engine."
	m, _, _ = m.Update(api.StreamChunkMsg{
		ContentDelta: longLine,
	})

	viewportView := m.Viewport.View()
	if len(viewportView) == 0 {
		t.Fatalf("expected non-empty viewport view")
	}

	// Verify that text is wrapped (contains newlines)
	if m.Viewport.TotalLineCount() <= 1 {
		t.Errorf("expected long line to be wrapped into multiple lines, total lines = %d", m.Viewport.TotalLineCount())
	}
}

func TestTesterModel_MarkdownRendering(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	rec := db.ProviderRecord{
		Name:    "Test Provider",
		BaseURL: "https://api.openai.com",
		APIKey:  "sk-test",
		APIType: api.APITypeOpenAIChat,
		Model:   "gpt-4o",
	}

	m := NewTesterModel(database, rec)
	m.Resize(80, 30)

	markdownText := "# Header\n\n- Item 1\n- Item 2\n\n```python\nprint('hello')\n```"
	m, _, _ = m.Update(api.StreamChunkMsg{
		ContentDelta: markdownText,
		Done:         true,
	})

	viewportView := m.Viewport.View()
	if len(viewportView) == 0 {
		t.Fatalf("expected non-empty viewport view after markdown rendering")
	}
}
