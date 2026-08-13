package views

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"llm_tui/internal/api"
	"llm_tui/internal/clipboard"
	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type ActivePane int

const (
	PaneRequest ActivePane = iota
	PaneResponse
)

type TesterModel struct {
	DB                     *db.DB
	Record                 db.ProviderRecord
	ReasoningEffort        string
	Textarea               textarea.Model
	Viewport               viewport.Model
	Spinner                spinner.Model
	ActivePane             ActivePane
	IsExecuting            bool
	IsStreamMode           bool
	SelectingModel         bool
	DiscoveredModels       []string
	ModelIndex             int
	LastResult             *api.TestResult
	CopyStatusMsg          string
	Width                  int
	Height                 int
	StreamChan             chan api.StreamChunkMsg
	ReasoningText          string
	ContentText            string
	StreamStatusCode       int
	StreamLatency          time.Duration
	StreamPromptTokens     int
	StreamCompletionTokens int
	StreamTotalTokens      int
	StreamError            string
	LastRawViewportContent string
}

type executeFinishedMsg struct {
	result *api.TestResult
}

type testerModelsFetchedMsg struct {
	models []string
}

func renderMarkdown(text string, width int) string {
	if text == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return text
	}
	rendered, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimSpace(rendered)
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

func (m *TesterModel) setViewportContent(content string) {
	m.LastRawViewportContent = content
	if content == "" {
		m.Viewport.SetContent("")
		return
	}
	if m.Viewport.Width > 0 {
		wrapped := lipgloss.NewStyle().Width(m.Viewport.Width).Render(content)
		m.Viewport.SetContent(wrapped)
	} else {
		m.Viewport.SetContent(content)
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

	if m.LastRawViewportContent != "" {
		m.setViewportContent(m.LastRawViewportContent)
	}
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
		m.setViewportContent(msg.result.FormattedBody)
		m.Viewport.GotoTop()
		m.CopyStatusMsg = ""
		return m, nil, ""

	case api.StreamChunkMsg:
		if msg.StatusCode != 0 {
			m.StreamStatusCode = msg.StatusCode
		}
		if msg.Latency != 0 {
			m.StreamLatency = msg.Latency
		}
		if msg.PromptTokens > 0 {
			m.StreamPromptTokens = msg.PromptTokens
		}
		if msg.CompletionTokens > 0 {
			m.StreamCompletionTokens = msg.CompletionTokens
		}
		if msg.TotalTokens > 0 {
			m.StreamTotalTokens = msg.TotalTokens
		}
		if msg.Err != nil {
			m.StreamError = msg.Err.Error()
		}

		if msg.ReasoningDelta != "" {
			m.ReasoningText += msg.ReasoningDelta
		}
		if msg.ContentDelta != "" {
			m.ContentText += msg.ContentDelta
		}

		reasoningText := m.ReasoningText
		contentText := m.ContentText

		var formattedContent string
		if reasoningText != "" {
			var rBuilder strings.Builder
			rBuilder.WriteString(styles.BadgeAccentStyle.Render("💭 Thinking Process") + "\n")
			rBuilder.WriteString(styles.HelpStyle.Render("-------------------") + "\n")

			if msg.Done {
				renderedReasoning := renderMarkdown(reasoningText, m.Viewport.Width)
				if renderedReasoning != "" {
					rBuilder.WriteString(renderedReasoning + "\n\n")
				} else {
					rBuilder.WriteString(reasoningText + "\n\n")
				}
			} else {
				rBuilder.WriteString(reasoningText + "\n\n")
			}

			rBuilder.WriteString(styles.BadgeSuccessStyle.Render("💬 Response Content") + "\n")
			rBuilder.WriteString(styles.HelpStyle.Render("-------------------") + "\n")

			if msg.Done {
				renderedContent := renderMarkdown(contentText, m.Viewport.Width)
				if renderedContent != "" {
					rBuilder.WriteString(renderedContent)
				} else {
					rBuilder.WriteString(contentText)
				}
			} else {
				rBuilder.WriteString(contentText)
			}
			formattedContent = rBuilder.String()
		} else if contentText != "" {
			if json.Valid([]byte(contentText)) {
				formattedContent = api.FormatJSON(contentText)
			} else {
				if msg.Done {
					renderedContent := renderMarkdown(contentText, m.Viewport.Width)
					if renderedContent != "" {
						formattedContent = renderedContent
					} else {
						formattedContent = contentText
					}
				} else {
					formattedContent = contentText
				}
			}
		} else if m.StreamError != "" {
			formattedContent = styles.ErrorStyle.Render("Error: " + m.StreamError)
		}

		m.setViewportContent(formattedContent)
		if m.IsExecuting {
			m.Viewport.GotoBottom()
		}

		if msg.Done {
			m.IsExecuting = false
			m.StreamChan = nil

			var assembledJSON string
			if reasoningText != "" || contentText != "" {
				assembledMap := map[string]interface{}{
					"model": m.Record.Model,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"message": map[string]interface{}{
								"role":              "assistant",
								"content":           contentText,
								"reasoning_content": reasoningText,
							},
							"finish_reason": "stop",
						},
					},
					"usage": map[string]int{
						"prompt_tokens":     m.StreamPromptTokens,
						"completion_tokens": m.StreamCompletionTokens,
						"total_tokens":      m.StreamTotalTokens,
					},
				}
				buf, err := json.Marshal(assembledMap)
				if err == nil {
					assembledJSON = string(buf)
				} else {
					assembledJSON = contentText
				}
			} else if m.StreamError != "" {
				assembledJSON = fmt.Sprintf(`{"error": %q}`, m.StreamError)
			}

			m.LastResult = &api.TestResult{
				StatusCode:       m.StreamStatusCode,
				Latency:          m.StreamLatency,
				PromptTokens:     m.StreamPromptTokens,
				CompletionTokens: m.StreamCompletionTokens,
				TotalTokens:      m.StreamTotalTokens,
				RawBody:          assembledJSON,
				FormattedBody:    api.FormatJSON(assembledJSON),
				Error:            m.StreamError,
			}
			m.CopyStatusMsg = ""

			m.setViewportContent(formattedContent)
			return m, nil, ""
		}

		return m, waitForStreamChunkCmd(m.StreamChan), ""

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
					m.Record.Name = newModel
					// Preserve current stream setting when regenerating template
					currentStream := true
					var oldPayload map[string]interface{}
					if err := json.Unmarshal([]byte(m.Textarea.Value()), &oldPayload); err == nil {
						if s, ok := oldPayload["stream"].(bool); ok {
							currentStream = s
						}
					}
					newTemplate := api.GeneratePayloadTemplate(m.Record.APIType, m.Record.Model, m.ReasoningEffort)
					var newPayload map[string]interface{}
					if err := json.Unmarshal([]byte(newTemplate), &newPayload); err == nil {
						newPayload["stream"] = currentStream
						if buf, err := json.MarshalIndent(newPayload, "", "  "); err == nil {
							m.Textarea.SetValue(string(buf))
						} else {
							m.Textarea.SetValue(newTemplate)
						}
					} else {
						m.Textarea.SetValue(newTemplate)
					}
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
			m.ReasoningText = ""
			m.ContentText = ""
			m.StreamStatusCode = 0
			m.StreamLatency = 0
			m.StreamPromptTokens = 0
			m.StreamCompletionTokens = 0
			m.StreamTotalTokens = 0
			m.StreamError = ""
			m.LastResult = nil
			m.setViewportContent("")

			m.Record.CustomPayload = m.Textarea.Value()
			m.Record.ReasoningEffort = m.ReasoningEffort
			_ = m.DB.UpdateRecord(&m.Record)

			// Parse stream property from request payload
			var payloadMap map[string]interface{}
			isStream := false
			if err := json.Unmarshal([]byte(m.Textarea.Value()), &payloadMap); err == nil {
				if s, ok := payloadMap["stream"].(bool); ok {
					isStream = s
				}
			} else {
				if strings.Contains(m.Textarea.Value(), `"stream": true`) || strings.Contains(m.Textarea.Value(), `"stream":true`) {
					isStream = true
				}
			}
			m.IsStreamMode = isStream

			if isStream {
				execCmd, ch := m.runExecuteStreamCmd()
				m.StreamChan = ch
				return m, tea.Batch(m.Spinner.Tick, execCmd), ""
			} else {
				return m, tea.Batch(m.Spinner.Tick, m.runExecuteNonStreamCmd()), ""
			}

		case "alt+s":
			// Toggle stream parameter in payload JSON
			var payloadMap map[string]interface{}
			if err := json.Unmarshal([]byte(m.Textarea.Value()), &payloadMap); err == nil {
				currentStream := false
				if s, ok := payloadMap["stream"].(bool); ok {
					currentStream = s
				}
				payloadMap["stream"] = !currentStream
				buf, err := json.MarshalIndent(payloadMap, "", "  ")
				if err == nil {
					m.Textarea.SetValue(string(buf))
				}
				if !currentStream {
					m.CopyStatusMsg = "🔄 Stream mode: ON"
				} else {
					m.CopyStatusMsg = "🔄 Stream mode: OFF"
				}
			} else {
				m.CopyStatusMsg = "❌ Cannot toggle stream: invalid JSON payload"
			}
			return m, nil, ""

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
			// Preserve current stream setting when regenerating template
			currentStream := true
			var oldPayload map[string]interface{}
			if err := json.Unmarshal([]byte(m.Textarea.Value()), &oldPayload); err == nil {
				if s, ok := oldPayload["stream"].(bool); ok {
					currentStream = s
				}
			}
			newTemplate := api.GeneratePayloadTemplate(m.Record.APIType, m.Record.Model, m.ReasoningEffort)
			var newPayload map[string]interface{}
			if err := json.Unmarshal([]byte(newTemplate), &newPayload); err == nil {
				newPayload["stream"] = currentStream
				if buf, err := json.MarshalIndent(newPayload, "", "  "); err == nil {
					m.Textarea.SetValue(string(buf))
				} else {
					m.Textarea.SetValue(newTemplate)
				}
			} else {
				m.Textarea.SetValue(newTemplate)
			}
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

func (m TesterModel) runExecuteNonStreamCmd() tea.Cmd {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	apiType := m.Record.APIType
	payload := m.Textarea.Value()

	return func() tea.Msg {
		res := api.ExecuteTestRequest(baseURL, apiKey, apiType, payload)
		return executeFinishedMsg{result: res}
	}
}

func (m *TesterModel) runExecuteStreamCmd() (tea.Cmd, chan api.StreamChunkMsg) {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	apiType := m.Record.APIType
	payload := m.Textarea.Value()

	ch := make(chan api.StreamChunkMsg, 100)
	go api.ExecuteStreamRequest(baseURL, apiKey, apiType, payload, ch)
	return waitForStreamChunkCmd(ch), ch
}

func waitForStreamChunkCmd(ch chan api.StreamChunkMsg) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return api.StreamChunkMsg{Done: true}
		}
		msg, ok := <-ch
		if !ok {
			return api.StreamChunkMsg{Done: true}
		}
		return msg
	}
}

func (m TesterModel) View() string {
	var sb strings.Builder

	header := styles.HeaderStyle.Render(fmt.Sprintf("🧪 LLM Chat Laboratory: %s (%s)", m.Record.Model, m.Record.APIType))
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
	if m.IsStreamMode {
		if m.ActivePane == PaneResponse {
			rightBorderColor = styles.ColorSecondary
			rightTitle = "📊 Response Stream (Active Scroll)"
		} else {
			rightBorderColor = styles.ColorMuted
			rightTitle = "📊 Response Stream"
		}
	} else {
		if m.ActivePane == PaneResponse {
			rightBorderColor = styles.ColorSecondary
			rightTitle = "📊 Response JSON (Active Scroll)"
		} else {
			rightBorderColor = styles.ColorMuted
			rightTitle = "📊 Response JSON"
		}
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
		if m.IsStreamMode {
			statusText := "HTTP ..."
			if m.StreamStatusCode > 0 {
				statusText = fmt.Sprintf("HTTP %d", m.StreamStatusCode)
			}
			metrics := fmt.Sprintf(
				"%s %s | Latency: %s\nTokens: P=%d, C=%d, T=%d",
				m.Spinner.View(),
				styles.BadgeSuccessStyle.Render(statusText),
				styles.MetricValueStyle.Render(fmt.Sprintf("%v", m.StreamLatency)),
				m.StreamPromptTokens,
				m.StreamCompletionTokens,
				m.StreamTotalTokens,
			)
			rightContentBuilder.WriteString(metrics + "\n\n")
			if m.StreamError != "" {
				streamErrSummary := m.StreamError
				if idx := strings.IndexAny(streamErrSummary, "\n\r"); idx > 0 {
					streamErrSummary = streamErrSummary[:idx]
				}
				if len(streamErrSummary) > 80 {
					streamErrSummary = streamErrSummary[:80] + "..."
				}
				rightContentBuilder.WriteString(styles.ErrorStyle.Render("Error: "+streamErrSummary) + "\n")
			}
			rightContentBuilder.WriteString(m.Viewport.View())
		} else {
			rightContentBuilder.WriteString("\n" + m.Spinner.View() + " Sending request payload...\n")
		}
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
			errSummary := m.LastResult.Error
			// Truncate to first line for display above viewport
			if idx := strings.IndexAny(errSummary, "\n\r"); idx > 0 {
				errSummary = errSummary[:idx]
			}
			if len(errSummary) > 80 {
				errSummary = errSummary[:80] + "..."
			}
			rightContentBuilder.WriteString(styles.ErrorStyle.Render("Error: "+errSummary) + "\n")
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
		"[Ctrl+S] Send  [Ctrl+Y] Copy Req  [Ctrl+U] Copy Resp  [PgUp/PgDn] Scroll  [Alt+M] Model  [Alt+S] Stream  [Tab] Pane  [Alt+1~4] Reasoning  [Esc] Manager",
	)
	sb.WriteString(helpKey)

	return sb.String()
}
