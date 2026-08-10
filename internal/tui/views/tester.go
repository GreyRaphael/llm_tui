package views

import (
	"fmt"
	"strings"

	"llm_tui/internal/api"
	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TestMode int

const (
	ModeEditPayload TestMode = iota
	ModeExecuting
	ModeViewResponse
)

type TesterModel struct {
	DB              *db.DB
	Record          db.ProviderRecord
	ReasoningEffort string
	Textarea        textarea.Model
	Viewport        viewport.Model
	Spinner         spinner.Model
	Mode            TestMode
	LastResult      *api.TestResult
	Width           int
	Height          int
}

type executeFinishedMsg struct {
	result *api.TestResult
}

func NewTesterModel(database *db.DB, record db.ProviderRecord) TesterModel {
	ta := textarea.New()
	ta.Placeholder = "Enter request JSON payload here..."
	ta.Focus()
	ta.SetWidth(70)
	ta.SetHeight(12)

	vp := viewport.New(70, 15)

	s := spinner.New()
	s.Spinner = spinner.Dot

	reasoning := record.ReasoningEffort
	if reasoning == "" {
		reasoning = db.ReasoningEffortNone
	}

	initialPayload := record.CustomPayload
	if initialPayload == "" {
		initialPayload = api.GeneratePayloadTemplate(record.APIType, record.Model, reasoning)
	}
	ta.SetValue(initialPayload)

	return TesterModel{
		DB:              database,
		Record:          record,
		ReasoningEffort: reasoning,
		Textarea:        ta,
		Viewport:        vp,
		Spinner:         s,
		Mode:            ModeEditPayload,
	}
}

func (m TesterModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m TesterModel) Update(msg tea.Msg) (TesterModel, tea.Cmd, string) {
	var action string
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case executeFinishedMsg:
		m.LastResult = msg.result
		m.Mode = ModeViewResponse
		m.Viewport.SetContent(msg.result.FormattedBody)
		return m, nil, ""

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			action = "back_to_manager"
			return m, nil, action

		case "ctrl+s", "ctrl+r":
			if m.Mode == ModeEditPayload || m.Mode == ModeViewResponse {
				m.Mode = ModeExecuting
				// Save payload as custom payload in SQLite
				m.Record.CustomPayload = m.Textarea.Value()
				m.Record.ReasoningEffort = m.ReasoningEffort
				_ = m.DB.UpdateRecord(&m.Record)

				return m, tea.Batch(m.Spinner.Tick, m.runExecuteCmd()), ""
			}

		case "1", "2", "3", "4":
			// Reasoning Effort Quick Shortcuts when in edit payload mode
			if m.Mode == ModeEditPayload {
				switch msg.String() {
				case "1":
					m.ReasoningEffort = db.ReasoningEffortNone
				case "2":
					m.ReasoningEffort = db.ReasoningEffortLow
				case "3":
					m.ReasoningEffort = db.ReasoningEffortMedium
				case "4":
					m.ReasoningEffort = db.ReasoningEffortHigh
				}
				// Regenerate payload template
				m.Textarea.SetValue(api.GeneratePayloadTemplate(m.Record.APIType, m.Record.Model, m.ReasoningEffort))
				return m, nil, ""
			}
		}
	}

	switch m.Mode {
	case ModeEditPayload:
		m.Textarea, cmd = m.Textarea.Update(msg)
	case ModeExecuting:
		m.Spinner, cmd = m.Spinner.Update(msg)
	case ModeViewResponse:
		m.Viewport, cmd = m.Viewport.Update(msg)
	}

	return m, cmd, action
}

func (m TesterModel) runExecuteCmd() tea.Cmd {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	apiType := m.Record.APIType
	payload := m.Textarea.Value()

	return func() tea.Msg {
		res := api.ExecuteTestRequest(baseURL, apiKey, apiType, payload)
		return executeFinishedMsg{result: res}
	}
}

func (m TesterModel) View() string {
	var sb strings.Builder

	header := styles.HeaderStyle.Render(fmt.Sprintf("🧪 LLM Chat Laboratory: %s (%s)", m.Record.Name, m.Record.Model))
	sb.WriteString(header + "\n\n")

	// Info Card
	info := fmt.Sprintf(
		"Base URL: %s | API Type: %s | Model: %s",
		m.Record.BaseURL,
		styles.BadgeSuccessStyle.Render(m.Record.APIType),
		styles.MetricValueStyle.Render(m.Record.Model),
	)
	sb.WriteString(styles.CardStyle.Render(info) + "\n\n")

	// Reasoning Effort Switcher
	sb.WriteString(styles.MetricLabelStyle.Render("Reasoning Effort Shortcuts: ") + " ")
	efforts := []string{db.ReasoningEffortNone, db.ReasoningEffortLow, db.ReasoningEffortMedium, db.ReasoningEffortHigh}
	for i, eff := range efforts {
		num := i + 1
		if eff == m.ReasoningEffort {
			sb.WriteString(styles.BadgeAccentStyle.Render(fmt.Sprintf("[%d] %s", num, eff)) + " ")
		} else {
			sb.WriteString(styles.BadgeStyle.Render(fmt.Sprintf("[%d] %s", num, eff)) + " ")
		}
	}
	sb.WriteString("\n\n")

	// Request JSON payload editor
	sb.WriteString(styles.SubtitleStyle.Render("Request Payload JSON (Editable):") + "\n")
	sb.WriteString(m.Textarea.View() + "\n\n")

	// Status / Response panel
	switch m.Mode {
	case ModeEditPayload:
		sb.WriteString(styles.HelpStyle.Render("[Ctrl+S] Send Request  [1-4] Set Reasoning Effort  [Esc] Back to Manager"))

	case ModeExecuting:
		sb.WriteString(m.Spinner.View() + " Sending single-turn payload request...\n")

	case ModeViewResponse:
		sb.WriteString(styles.SubtitleStyle.Render("Response Analysis:") + "\n")
		if m.LastResult != nil {
			var statusStyle lipgloss.Style
			if m.LastResult.StatusCode == 200 {
				statusStyle = styles.BadgeSuccessStyle
			} else {
				statusStyle = styles.BadgeAccentStyle
			}

			metrics := fmt.Sprintf(
				"%s | Latency: %s | Tokens: Prompt=%d, Completion=%d, Total=%d",
				statusStyle.Render(fmt.Sprintf("HTTP %d", m.LastResult.StatusCode)),
				styles.MetricValueStyle.Render(fmt.Sprintf("%v", m.LastResult.Latency)),
				m.LastResult.PromptTokens,
				m.LastResult.CompletionTokens,
				m.LastResult.TotalTokens,
			)
			sb.WriteString(styles.CardStyle.Render(metrics) + "\n")
			if m.LastResult.Error != "" {
				sb.WriteString(styles.ErrorStyle.Render("Error: "+m.LastResult.Error) + "\n")
			}
			sb.WriteString(m.Viewport.View() + "\n\n")
		}
		sb.WriteString(styles.HelpStyle.Render("[Ctrl+S] Re-send Request  [1-4] Change Reasoning Effort  [Esc] Back to Manager"))
	}

	return sb.String()
}
