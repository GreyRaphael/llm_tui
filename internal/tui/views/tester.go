package views

import (
	"fmt"
	"strings"

	"llm_tui/internal/api"
	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ActivePane int

const (
	PaneRequest ActivePane = iota
	PaneResponse
)

type TesterModel struct {
	DB               *db.DB
	Record           db.ProviderRecord
	ReasoningEffort  string
	Textarea         textarea.Model
	Viewport         viewport.Model
	Spinner          spinner.Model
	ActivePane       ActivePane
	IsExecuting      bool
	SelectingModel   bool
	DiscoveredModels []string
	ModelIndex       int
	LastResult       *api.TestResult
	CopyStatusMsg    string
	Width            int
	Height           int
}

type executeFinishedMsg struct {
	result *api.TestResult
}

type testerModelsFetchedMsg struct {
	models []string
}

func NewTesterModel(database *db.DB, record db.ProviderRecord) TesterModel {
	ta := textarea.New()
	ta.Placeholder = "Enter request JSON payload here..."
	ta.Focus()
	ta.SetWidth(40)
	ta.SetHeight(16)

	vp := viewport.New(40, 16)

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
		ActivePane:      PaneRequest,
	}
}

func (m *TesterModel) Resize(w, h int) {
	m.Width = w
	m.Height = h

	totalWidth := w - 4
	halfWidth := (totalWidth - 2) / 2
	if halfWidth < 35 {
		halfWidth = 40
	}

	paneHeight := h - 13
	if paneHeight < 12 {
		paneHeight = 16
	}

	m.Textarea.SetWidth(halfWidth - 4)
	m.Textarea.SetHeight(paneHeight - 2)

	m.Viewport.Width = halfWidth - 4
	m.Viewport.Height = paneHeight - 5
}

func (m TesterModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.runFetchModelsCmd())
}

func (m TesterModel) Update(msg tea.Msg) (TesterModel, tea.Cmd, string) {
	var action string
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case testerModelsFetchedMsg:
		m.DiscoveredModels = msg.models
		for i, md := range m.DiscoveredModels {
			if md == m.Record.Model {
				m.ModelIndex = i
				break
			}
		}
		return m, nil, ""

	case executeFinishedMsg:
		m.IsExecuting = false
		m.LastResult = msg.result
		m.Viewport.SetContent(msg.result.FormattedBody)
		m.Viewport.GotoTop()
		m.CopyStatusMsg = ""
		return m, nil, ""

	case tea.KeyMsg:
		if m.SelectingModel {
			switch msg.String() {
			case "esc":
				m.SelectingModel = false
				return m, nil, ""
			case "up", "k":
				if m.ModelIndex > 0 {
					m.ModelIndex--
				}
				return m, nil, ""
			case "down", "j":
				if m.ModelIndex < len(m.DiscoveredModels)-1 {
					m.ModelIndex++
				}
				return m, nil, ""
			case "enter":
				if len(m.DiscoveredModels) > 0 && m.ModelIndex < len(m.DiscoveredModels) {
					newModel := m.DiscoveredModels[m.ModelIndex]
					m.Record.Model = newModel
					m.Record.Name = fmt.Sprintf("%s (%s)", newModel, m.Record.APIType)
					m.Textarea.SetValue(api.GeneratePayloadTemplate(m.Record.APIType, m.Record.Model, m.ReasoningEffort))
					m.Record.CustomPayload = m.Textarea.Value()
					_ = m.DB.UpdateRecord(&m.Record)
					m.CopyStatusMsg = fmt.Sprintf("Switched model to '%s'", newModel)
				}
				m.SelectingModel = false
				return m, nil, ""
			}
			return m, nil, ""
		}

		switch msg.String() {
		case "esc":
			action = "back_to_manager"
			return m, nil, action

		case "alt+m":
			if len(m.DiscoveredModels) > 0 {
				m.SelectingModel = true
			} else {
				m.CopyStatusMsg = "No models list available via /models for this provider"
			}
			return m, nil, ""

		case "tab", "shift+tab":
			if m.ActivePane == PaneRequest {
				m.ActivePane = PaneResponse
				m.Textarea.Blur()
			} else {
				m.ActivePane = PaneRequest
				m.Textarea.Focus()
			}
			return m, textarea.Blink, ""

		// Copy Request JSON with Ctrl+Y
		case "ctrl+y":
			reqPayload := m.Textarea.Value()
			if reqPayload != "" {
				err := clipboard.WriteAll(reqPayload)
				if err == nil {
					m.CopyStatusMsg = "📋 Request Payload JSON copied to clipboard!"
				} else {
					m.CopyStatusMsg = fmt.Sprintf("❌ Copy failed: %v", err)
				}
				return m, nil, ""
			}

		// Copy Response JSON with Ctrl+U
		case "ctrl+u":
			if m.LastResult != nil && m.LastResult.FormattedBody != "" {
				err := clipboard.WriteAll(m.LastResult.FormattedBody)
				if err == nil {
					m.CopyStatusMsg = "📋 Response JSON copied to clipboard!"
				} else {
					m.CopyStatusMsg = fmt.Sprintf("❌ Copy failed: %v", err)
				}
				return m, nil, ""
			}

		// Page Down / Page Up scrolling for Response Viewport
		case "pgdown", "pagedown", "ctrl+f":
			if m.ActivePane == PaneResponse {
				m.Viewport.HalfViewDown()
				return m, nil, ""
			}
		case "pgup", "pageup", "ctrl+b":
			if m.ActivePane == PaneResponse {
				m.Viewport.HalfViewUp()
				return m, nil, ""
			}

		case "ctrl+s", "ctrl+r":
			m.IsExecuting = true
			m.CopyStatusMsg = ""
			m.Record.CustomPayload = m.Textarea.Value()
			m.Record.ReasoningEffort = m.ReasoningEffort
			_ = m.DB.UpdateRecord(&m.Record)

			return m, tea.Batch(m.Spinner.Tick, m.runExecuteCmd()), ""

		case "alt+1", "alt+2", "alt+3", "alt+4":
			switch msg.String() {
			case "alt+1":
				m.ReasoningEffort = db.ReasoningEffortNone
			case "alt+2":
				m.ReasoningEffort = db.ReasoningEffortLow
			case "alt+3":
				m.ReasoningEffort = db.ReasoningEffortHigh
			case "alt+4":
				m.ReasoningEffort = db.ReasoningEffortMax
			}
			m.Textarea.SetValue(api.GeneratePayloadTemplate(m.Record.APIType, m.Record.Model, m.ReasoningEffort))
			return m, nil, ""
		}
	}

	if m.IsExecuting {
		m.Spinner, cmd = m.Spinner.Update(msg)
	} else if m.ActivePane == PaneRequest {
		m.Textarea, cmd = m.Textarea.Update(msg)
	} else {
		m.Viewport, cmd = m.Viewport.Update(msg)
	}

	return m, cmd, action
}

func (m TesterModel) runFetchModelsCmd() tea.Cmd {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	return func() tea.Msg {
		models, _ := api.FetchModels(baseURL, apiKey)
		return testerModelsFetchedMsg{models: models}
	}
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

	header := styles.HeaderStyle.Render(fmt.Sprintf("🧪 LLM Chat Laboratory: %s", m.Record.Name))
	sb.WriteString(header + "\n\n")

	// Info Card with Model Switcher Badge
	modelBadge := fmt.Sprintf("Model: %s [Press 'm' to switch]", m.Record.Model)
	info := fmt.Sprintf(
		"Base URL: %s | API Type: %s | %s",
		m.Record.BaseURL,
		styles.BadgeSuccessStyle.Render(m.Record.APIType),
		styles.MetricValueStyle.Render(modelBadge),
	)
	sb.WriteString(styles.CardStyle.Render(info) + "\n")

	// Reasoning Effort Switcher
	sb.WriteString(styles.MetricLabelStyle.Render("Reasoning Effort Shortcuts: ") + " ")
	efforts := []string{db.ReasoningEffortNone, db.ReasoningEffortLow, db.ReasoningEffortHigh, db.ReasoningEffortMax}
	for i, eff := range efforts {
		num := i + 1
		if eff == m.ReasoningEffort {
			sb.WriteString(styles.BadgeAccentStyle.Render(fmt.Sprintf("[%d] %s", num, eff)) + " ")
		} else {
			sb.WriteString(styles.BadgeStyle.Render(fmt.Sprintf("[%d] %s", num, eff)) + " ")
		}
	}
	sb.WriteString("\n\n")

	// Calculate Pane Dimensions
	totalWidth := m.Width - 4
	halfWidth := (totalWidth - 2) / 2
	if halfWidth < 35 {
		halfWidth = 40
	}
	paneHeight := m.Height - 13
	if paneHeight < 12 {
		paneHeight = 16
	}

	// 1. Render Left Pane (Request Payload Editor OR Model Picker Overlay)
	var leftBorderColor lipgloss.Color
	var leftTitle string
	var leftContent string

	if m.SelectingModel {
		leftBorderColor = styles.ColorAccent
		leftTitle = "🤖 Select Model (↑/↓ to move, Enter to apply, Esc to cancel)"

		var modelContentBuilder strings.Builder
		modelContentBuilder.WriteString(styles.SubtitleStyle.Render(leftTitle) + "\n\n")

		totalModels := len(m.DiscoveredModels)
		maxVisible := paneHeight - 4
		if maxVisible < 20 {
			maxVisible = 20
		}

		startIdx := m.ModelIndex - maxVisible/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + maxVisible
		if endIdx > totalModels {
			endIdx = totalModels
			startIdx = endIdx - maxVisible
			if startIdx < 0 {
				startIdx = 0
			}
		}

		if startIdx > 0 {
			modelContentBuilder.WriteString(fmt.Sprintf("  ▲ %d models above...\n", startIdx))
		}

		for i := startIdx; i < endIdx; i++ {
			prefix := "  "
			if i == m.ModelIndex {
				prefix = "👉"
			}
			modelContentBuilder.WriteString(fmt.Sprintf("%s %s\n", prefix, styles.MetricValueStyle.Render(m.DiscoveredModels[i])))
		}

		if endIdx < totalModels {
			modelContentBuilder.WriteString(fmt.Sprintf("  ▼ %d models below...\n", totalModels-endIdx))
		}

		leftContent = modelContentBuilder.String()

	} else {
		if m.ActivePane == PaneRequest {
			leftBorderColor = styles.ColorSecondary
			leftTitle = "📝 Request Payload JSON (Active Focus)"
		} else {
			leftBorderColor = styles.ColorMuted
			leftTitle = "📝 Request Payload JSON"
		}
		leftContent = fmt.Sprintf("%s\n%s", styles.SubtitleStyle.Render(leftTitle), m.Textarea.View())
	}

	leftBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(leftBorderColor).
		Padding(0, 1).
		Width(halfWidth).
		Height(paneHeight)

	leftPaneView := leftBoxStyle.Render(leftContent)

	// 2. Render Right Pane (Response Panel)
	var rightBorderColor lipgloss.Color
	var rightTitle string
	if m.ActivePane == PaneResponse {
		rightBorderColor = styles.ColorSecondary
		rightTitle = "📊 Response JSON (Active Scroll)"
	} else {
		rightBorderColor = styles.ColorMuted
		rightTitle = "📊 Response JSON"
	}

	rightBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rightBorderColor).
		Padding(0, 1).
		Width(halfWidth).
		Height(paneHeight)

	var rightContentBuilder strings.Builder
	rightContentBuilder.WriteString(styles.SubtitleStyle.Render(rightTitle) + "\n")

	if m.CopyStatusMsg != "" {
		rightContentBuilder.WriteString(styles.BadgeSuccessStyle.Render(m.CopyStatusMsg) + "\n")
	}

	if m.IsExecuting {
		rightContentBuilder.WriteString("\n" + m.Spinner.View() + " Sending request payload...\n")
	} else if m.LastResult != nil {
		var statusStyle lipgloss.Style
		statusText := fmt.Sprintf("HTTP %d", m.LastResult.StatusCode)
		if m.LastResult.StatusCode == 200 && m.LastResult.Error == "" {
			statusStyle = styles.BadgeSuccessStyle
		} else {
			statusStyle = styles.BadgeAccentStyle
			if m.LastResult.Error != "" {
				statusText = fmt.Sprintf("HTTP %d (API Error)", m.LastResult.StatusCode)
			}
		}

		metrics := fmt.Sprintf(
			"%s | Latency: %s\nTokens: P=%d, C=%d, T=%d",
			statusStyle.Render(statusText),
			styles.MetricValueStyle.Render(fmt.Sprintf("%v", m.LastResult.Latency)),
			m.LastResult.PromptTokens,
			m.LastResult.CompletionTokens,
			m.LastResult.TotalTokens,
		)
		rightContentBuilder.WriteString(metrics + "\n\n")
		if m.LastResult.Error != "" {
			rightContentBuilder.WriteString(styles.ErrorStyle.Render("Error: "+m.LastResult.Error) + "\n")
		}
		rightContentBuilder.WriteString(m.Viewport.View())
	} else {
		rightContentBuilder.WriteString(lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("\nPress [Ctrl+S] to send payload and view response here."))
	}

	rightPaneView := rightBoxStyle.Render(rightContentBuilder.String())

	// Join Columns Side-by-Side
	splitView := lipgloss.JoinHorizontal(lipgloss.Top, leftPaneView, " ", rightPaneView)
	sb.WriteString(splitView + "\n")

	// Help Footer
	helpKey := styles.HelpStyle.Render(
		"[Ctrl+S] Send  [Ctrl+Y] Copy Req  [Ctrl+U] Copy Resp  [PgUp/PgDn] Scroll  [Alt+M] Model  [Tab] Pane  [Alt+1~4] Reasoning  [Esc] Manager",
	)
	sb.WriteString(helpKey)

	return sb.String()
}
