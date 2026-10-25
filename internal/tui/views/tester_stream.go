package views

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"llm_tui/internal/api"
	"llm_tui/internal/tui/styles"

	tea "github.com/charmbracelet/bubbletea"
)

func (m TesterModel) handleStreamChunk(msg api.StreamChunkMsg) (TesterModel, tea.Cmd, string) {
	m.applyStreamChunk(msg)
	if !msg.Done {
		if m.LastStreamRender.IsZero() || time.Since(m.LastStreamRender) >= 33*time.Millisecond {
			m.setViewportContent(m.renderStreamingProgress())
			if m.IsExecuting {
				m.Viewport.GotoBottom()
			}
			m.LastStreamRender = time.Now()
		}
		return m, waitForStreamChunkCmd(m.requestID, m.StreamChan), ""
	}

	m.IsExecuting = false
	m.StreamChan = nil
	cancelTrackedRequest(&m.cancelExecution, &m.requestID)
	m.setViewportContent(m.renderCompletedStream())
	m.LastResult = m.buildStreamResult()
	m.setStatus("", false)
	m.Viewport.GotoBottom()
	return m, nil, ""
}

func (m *TesterModel) applyStreamChunk(msg api.StreamChunkMsg) {
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
	m.ReasoningText += msg.ReasoningDelta
	m.ContentText += msg.ContentDelta
}

func (m TesterModel) renderStreamingProgress() string {
	if m.ReasoningText == "" {
		if m.ContentText != "" {
			return m.ContentText
		}
		if m.StreamError != "" {
			return styles.ErrorStyle.Render("Error: " + m.StreamError)
		}
		return ""
	}
	var sb strings.Builder
	sb.WriteString(styles.BadgeAccentStyle.Render("💭 Thinking Process") + "\n")
	sb.WriteString(styles.HelpStyle.Render("-------------------") + "\n")
	sb.WriteString(m.ReasoningText + "\n\n")
	sb.WriteString(styles.BadgeSuccessStyle.Render("💬 Response Content") + "\n")
	sb.WriteString(styles.HelpStyle.Render("-------------------") + "\n")
	sb.WriteString(m.ContentText)
	return sb.String()
}

func (m TesterModel) renderCompletedStream() string {
	if m.ReasoningText == "" {
		if m.ContentText != "" {
			if json.Valid([]byte(m.ContentText)) {
				return api.FormatJSON(m.ContentText)
			}
			if rendered := renderMarkdown(m.ContentText, m.Viewport.Width); rendered != "" {
				return rendered
			}
			return m.ContentText
		}
		if m.StreamError != "" {
			return styles.ErrorStyle.Render("Error: " + m.StreamError)
		}
		return ""
	}

	var sb strings.Builder
	sb.WriteString(styles.BadgeAccentStyle.Render("💭 Thinking Process") + "\n")
	sb.WriteString(styles.HelpStyle.Render("-------------------") + "\n")
	if rendered := renderMarkdown(m.ReasoningText, m.Viewport.Width); rendered != "" {
		sb.WriteString(rendered + "\n\n")
	} else {
		sb.WriteString(m.ReasoningText + "\n\n")
	}
	sb.WriteString(styles.BadgeSuccessStyle.Render("💬 Response Content") + "\n")
	sb.WriteString(styles.HelpStyle.Render("-------------------") + "\n")
	if rendered := renderMarkdown(m.ContentText, m.Viewport.Width); rendered != "" {
		sb.WriteString(rendered)
	} else {
		sb.WriteString(m.ContentText)
	}
	return sb.String()
}

func (m TesterModel) buildStreamResult() *api.TestResult {
	assembled := m.assembledStreamJSON()
	return &api.TestResult{
		StatusCode:       m.StreamStatusCode,
		Latency:          m.StreamLatency,
		PromptTokens:     m.StreamPromptTokens,
		CompletionTokens: m.StreamCompletionTokens,
		TotalTokens:      m.StreamTotalTokens,
		RawBody:          assembled,
		FormattedBody:    api.FormatJSON(assembled),
		Error:            m.StreamError,
	}
}

func (m TesterModel) assembledStreamJSON() string {
	if m.ReasoningText == "" && m.ContentText == "" {
		if m.StreamError != "" {
			return fmt.Sprintf(`{"error": %q}`, m.StreamError)
		}
		return ""
	}
	payload := map[string]interface{}{
		"model": m.Record.Model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           m.ContentText,
					"reasoning_content": m.ReasoningText,
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
	buf, err := json.Marshal(payload)
	if err != nil {
		return m.ContentText
	}
	return string(buf)
}
