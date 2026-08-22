package views

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
		{"sk-12345", "******"},       // exactly 8 chars
		{"sk-123456", "sk-1...3456"}, // 9 chars: first 4 + ... + last 4
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

func TestFormatTPS(t *testing.T) {
	tests := []struct {
		name        string
		totalTokens int
		latency     time.Duration
		want        string
	}{
		{"normal", 211, 2623553075 * time.Nanosecond, "80.43"},
		{"zero tokens", 0, 2 * time.Second, "-"},
		{"zero latency", 100, 0, "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTPS(tt.totalTokens, tt.latency); got != tt.want {
				t.Errorf("formatTPS(%d, %v) = %q; want %q", tt.totalTokens, tt.latency, got, tt.want)
			}
		})
	}
}

func TestFormatLatency(t *testing.T) {
	tests := []struct {
		name    string
		latency time.Duration
		want    string
	}{
		{"normal", 2623553075 * time.Nanosecond, "2.62s"},
		{"zero", 0, "0.00s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatLatency(tt.latency); got != tt.want {
				t.Errorf("formatLatency(%v) = %q; want %q", tt.latency, got, tt.want)
			}
		})
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
		ContentDelta:     "Hello World!",
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
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

	// Let the first request finish so a second send is not rejected by the
	// in-flight guard (Ctrl+S while a request is running is intentionally
	// ignored).
	m.IsExecuting = false

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

func TestProbeModel_AutofillRespectsManualAPIKeyEdit(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	_ = database.CreateRecord(&db.ProviderRecord{
		Name:    "Saved",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-saved",
		APIType: "openai_chat",
		Model:   "gpt-4o",
	})

	m := NewProbeModel(database)
	m.BaseURLInput.SetValue("https://api.example.com")

	// Auto-fills when the key field is empty.
	m.checkAutofillAPIKey()
	if got := m.APIKeyInput.Value(); got != "sk-saved" {
		t.Fatalf("expected auto-filled key, got %q", got)
	}

	// A manual edit must never be overwritten by the autofill.
	m.APIKeyInput.SetValue("sk-manual")
	m.APIKeyEdited = true
	m.checkAutofillAPIKey()
	if got := m.APIKeyInput.Value(); got != "sk-manual" {
		t.Fatalf("manual key was overwritten by autofill: got %q", got)
	}

	// Moving to a Base URL with no saved record should clear a stale auto-filled
	// key (leak prevention) ...
	m.APIKeyEdited = false
	m.APIKeyInput.SetValue("")
	m.BaseURLInput.SetValue("https://other.example.com")
	m.checkAutofillAPIKey()
	if got := m.APIKeyInput.Value(); got != "" {
		t.Fatalf("expected stale auto-filled key cleared, got %q", got)
	}

	// ... but preserve an explicitly typed key.
	m.APIKeyEdited = true
	m.APIKeyInput.SetValue("sk-user")
	m.BaseURLInput.SetValue("https://third.example.com")
	m.checkAutofillAPIKey()
	if got := m.APIKeyInput.Value(); got != "sk-user" {
		t.Fatalf("manual key should be preserved, got %q", got)
	}
}

func TestProbeModel_TypingInAPIKeyFieldMarksEdit(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewProbeModel(database)

	// Tab to the API key field and type a character.
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.FocusIndex != 1 {
		t.Fatalf("expected focus on API key field after Tab, got %d", m.FocusIndex)
	}
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !m.APIKeyEdited {
		t.Fatal("expected APIKeyEdited to be true after typing in the key field")
	}
}

func TestTesterModel_IgnoresStaleStreamChunks(t *testing.T) {
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
	m.StreamID = 7 // emulate a current send that reached stream #7

	// A chunk tagged with an old stream id must be ignored entirely: it must
	// not touch content, executing state, or LastResult.
	m, _, _ = m.Update(streamChunkMsg{id: 6, msg: api.StreamChunkMsg{ContentDelta: "stale", Done: true}})
	if m.ContentText != "" {
		t.Fatalf("expected stale chunk ignored, got %q", m.ContentText)
	}
	if m.IsExecuting {
		t.Fatal("stale Done must not change executing state")
	}
	if m.LastResult != nil {
		t.Fatal("stale Done must not populate LastResult")
	}

	// A chunk tagged with the current stream id is processed normally.
	m.IsExecuting = true
	m, _, _ = m.Update(streamChunkMsg{id: 7, msg: api.StreamChunkMsg{ContentDelta: "fresh", Done: true}})
	if m.ContentText != "fresh" {
		t.Fatalf("expected current chunk accepted, got %q", m.ContentText)
	}
	if m.IsExecuting {
		t.Fatal("expected IsExecuting false after current stream done")
	}
	if m.LastResult == nil || !strings.Contains(m.LastResult.RawBody, "fresh") {
		t.Fatalf("expected LastResult built for current stream, got %+v", m.LastResult)
	}
}

func TestTesterModel_RejectsSendWhileExecuting(t *testing.T) {
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
	m.IsExecuting = true
	m.StreamID = 3

	// Ctrl+S while a request is running must be rejected: no new stream id is
	// allocated, executing state is preserved, and the user gets a hint.
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if !m.IsExecuting {
		t.Fatal("IsExecuting must stay true when a send is rejected")
	}
	if m.StreamID != 3 {
		t.Fatalf("rejected send must not allocate a new stream id, got %d", m.StreamID)
	}
	if m.CopyStatusMsg == "" {
		t.Fatal("expected a status hint after rejected send")
	}
}

func TestTesterModel_DBUpdateErrorFeedback(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}

	rec := db.ProviderRecord{
		Name:    "Test",
		BaseURL: "https://api.example.com",
		APIKey:  "sk-test",
		APIType: api.APITypeOpenAIChat,
		Model:   "gpt-4o",
	}
	_ = database.CreateRecord(&rec)

	m := NewTesterModel(database, rec)
	m.DiscoveredModels = []string{"gpt-4o", "gpt-4o-mini"}
	m.ModelIndex = 1
	m.SelectingModel = true

	// Close database so next UpdateRecord fails
	_ = database.Close()

	// Press Enter to switch model with closed DB
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.CopyStatusMsg, "Failed to save") {
		t.Fatalf("expected DB failure status message, got: %q", m.CopyStatusMsg)
	}
}

func TestManagerModel_ViewportScrolling(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	// Insert 10 provider records
	for i := 1; i <= 10; i++ {
		_ = database.CreateRecord(&db.ProviderRecord{
			Name:    fmt.Sprintf("Provider %02d", i),
			BaseURL: "https://api.example.com",
			APIKey:  "sk-test",
			APIType: api.APITypeOpenAIChat,
			Model:   "gpt-4o",
		})
	}

	m := NewManagerModel(database)
	m.Resize(80, 15) // small height: viewport height is around 9 lines (holds ~1.5 cards)

	if len(m.Records) != 10 {
		t.Fatalf("expected 10 records, got %d", len(m.Records))
	}
	if m.Viewport.YOffset != 0 {
		t.Fatalf("expected initial YOffset 0, got %d", m.Viewport.YOffset)
	}

	// Navigate down 6 times to item 6
	for i := 0; i < 6; i++ {
		m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	if m.Cursor != 6 {
		t.Fatalf("expected cursor at 6, got %d", m.Cursor)
	}
	if m.Viewport.YOffset == 0 {
		t.Fatalf("expected viewport to have scrolled down (YOffset > 0), got %d", m.Viewport.YOffset)
	}

	// Navigate back up to item 0
	for i := 0; i < 6; i++ {
		m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	}
	if m.Cursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", m.Cursor)
	}
	if m.Viewport.YOffset != 0 {
		t.Fatalf("expected viewport to scroll back to top (YOffset 0), got %d", m.Viewport.YOffset)
	}

	// Verify View contains record indicator
	viewOutput := m.View()
	if !strings.Contains(viewOutput, "[1/10]") {
		t.Fatalf("expected [1/10] in view output, got: %s", viewOutput)
	}
}

func TestTesterModel_StreamThrottling(t *testing.T) {
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
	m.Resize(80, 25)
	m.IsExecuting = true

	// Send rapid stream chunks
	chunks := []string{"Hello", " world", " from", " throttled", " stream"}
	for _, c := range chunks {
		m, _, _ = m.Update(api.StreamChunkMsg{ContentDelta: c})
	}

	if m.ContentText != "Hello world from throttled stream" {
		t.Fatalf("expected full accumulated content text, got %q", m.ContentText)
	}

	// Finalize stream with Done: true
	m, _, _ = m.Update(api.StreamChunkMsg{ContentDelta: "!", Done: true})
	if m.IsExecuting {
		t.Fatal("expected IsExecuting to be false after Done: true")
	}
	if m.ContentText != "Hello world from throttled stream!" {
		t.Fatalf("expected final content text, got %q", m.ContentText)
	}
	if m.LastResult == nil {
		t.Fatal("expected LastResult populated after stream completion")
	}
	if !strings.Contains(m.LastResult.RawBody, "Hello world from throttled stream!") {
		t.Fatalf("expected LastResult to contain complete response, got: %s", m.LastResult.RawBody)
	}
}

func TestProbeModel_ModelListLengthDisplay(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init memory db: %v", err)
	}
	defer database.Close()

	m := NewProbeModel(database)
	m.Resize(100, 30)

	// 1. When models are fetched successfully
	discovered := []string{"gpt-4o", "gpt-4o-mini", "o3-mini"}
	m, _, _ = m.Update(modelsFetchedMsg{models: discovered})

	if !strings.Contains(m.StatusMsg, "Model list length: 3") {
		t.Errorf("expected StatusMsg to contain 'Model list length: 3', got %q", m.StatusMsg)
	}

	viewOutput := m.View()
	if !strings.Contains(viewOutput, "Model list length: 3") {
		t.Errorf("expected View to contain 'Model list length: 3', got %q", viewOutput)
	}
	if !strings.Contains(viewOutput, "[1/3]") {
		t.Errorf("expected View to contain '[1/3]', got %q", viewOutput)
	}

	// 2. When no models are fetched
	mEmpty := NewProbeModel(database)
	mEmpty.Resize(100, 30)
	mEmpty, _, _ = mEmpty.Update(modelsFetchedMsg{models: nil})

	if !strings.Contains(mEmpty.StatusMsg, "Model list length: 0") {
		t.Errorf("expected empty StatusMsg to contain 'Model list length: 0', got %q", mEmpty.StatusMsg)
	}

	emptyViewOutput := mEmpty.View()
	if !strings.Contains(emptyViewOutput, "Model list length: 0") {
		t.Errorf("expected empty View to contain 'Model list length: 0', got %q", emptyViewOutput)
	}
}

func TestTesterModel_ModelListLengthDisplay(t *testing.T) {
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
	_ = database.CreateRecord(&rec)

	m := NewTesterModel(database, rec)
	m.Resize(120, 35)

	// 1. Initial view without fetched models shows length 0 in badge
	initialView := m.View()
	if !strings.Contains(initialView, "Model list length: 0") {
		t.Errorf("expected initial view badge to contain 'Model list length: 0', got %q", initialView)
	}

	// 2. Models fetched message
	discovered := []string{"gpt-4o", "gpt-4o-mini", "o3-mini"}
	m, _, _ = m.Update(testerModelsFetchedMsg{models: discovered})

	if !strings.Contains(m.CopyStatusMsg, "Model list length: 3") {
		t.Errorf("expected CopyStatusMsg after fetch to contain 'Model list length: 3', got %q", m.CopyStatusMsg)
	}

	fetchedView := m.View()
	if !strings.Contains(fetchedView, "Model list length: 3") {
		t.Errorf("expected info badge after fetch to contain 'Model list length: 3', got %q", fetchedView)
	}

	// 3. Open model selector via Alt+M
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
	if !m.SelectingModel {
		t.Fatal("expected SelectingModel to be true")
	}

	selectingView := m.View()
	if !strings.Contains(selectingView, "Model list length: 3") {
		t.Errorf("expected model selector title to contain 'Model list length: 3', got %q", selectingView)
	}
	if !strings.Contains(selectingView, "[1/3]") {
		t.Errorf("expected model selector title to contain '[1/3]', got %q", selectingView)
	}

	// 4. Move down and apply model switch on Enter
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m, _, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.SelectingModel {
		t.Fatal("expected SelectingModel to be false after Enter")
	}
	if !strings.Contains(m.CopyStatusMsg, "Switched model to 'gpt-4o-mini' (Model list length: 3)") {
		t.Errorf("expected switch status message with model list length, got %q", m.CopyStatusMsg)
	}

	// 5. When no models discovered and Alt+M pressed
	mNoModels := NewTesterModel(database, rec)
	mNoModels, _, _ = mNoModels.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
	if !strings.Contains(mNoModels.CopyStatusMsg, "Model list length: 0") {
		t.Errorf("expected Alt+M failure message to contain 'Model list length: 0', got %q", mNoModels.CopyStatusMsg)
	}
}

