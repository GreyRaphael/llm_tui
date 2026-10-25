package views

import (
	"encoding/json"
	"fmt"
	"strings"

	"llm_tui/internal/api"
	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	"github.com/charmbracelet/lipgloss"
)

func (m TesterModel) View() string {
	var sb strings.Builder
	sb.WriteString(m.renderHeader())
	halfWidth, paneHeight := m.paneDimensions()
	leftPane := m.renderLeftPane(halfWidth, paneHeight)
	rightPane := m.renderRightPane(halfWidth, paneHeight)
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane) + "\n")
	sb.WriteString(m.renderHelp())
	return sb.String()
}

func (m TesterModel) renderHeader() string {
	var sb strings.Builder
	title := fmt.Sprintf("🧪 LLM Chat Laboratory: %s (%s)", m.Record.Model, m.Record.APIType)
	if m.Record.APIType == api.APITypeOpenAIImages {
		title = fmt.Sprintf("🖼️ AI Image Laboratory: %s (%s)", m.Record.Model, m.Record.APIType)
	}
	sb.WriteString(styles.HeaderStyle.Render(title) + "\n\n")

	modelBadge := fmt.Sprintf("Model: %s [Press Alt+M to switch (Model list length: %d)]", m.Record.Model, len(m.DiscoveredModels))
	info := fmt.Sprintf(
		"Base URL: %s | API Type: %s | %s%s",
		m.Record.BaseURL,
		styles.BadgeSuccessStyle.Render(m.Record.APIType),
		styles.MetricValueStyle.Render(modelBadge),
		m.imageSizeBadge(),
	)
	sb.WriteString(styles.CardStyle.Render(info) + "\n")
	if m.Record.APIType == api.APITypeOpenAIImages {
		sb.WriteString(styles.HelpStyle.Render("📁 Output directory: ./generated_images/ | Press [Ctrl+O] to open last generated image in system viewer") + "\n")
	} else {
		sb.WriteString(m.reasoningEffortBadges())
	}
	sb.WriteString("\n\n")
	return sb.String()
}

func (m TesterModel) imageSizeBadge() string {
	if m.Record.APIType != api.APITypeOpenAIImages {
		return ""
	}
	current := "512x512 (1:1 0.5K)"
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(m.Textarea.Value()), &payload); err == nil {
		if size, ok := payload["size"].(string); ok && size != "" {
			current = size
			for _, option := range SupportedImageSizes {
				if strings.EqualFold(option.Size, size) {
					current = fmt.Sprintf("%s (%s %s)", option.Size, option.Ratio, option.Tier)
					break
				}
			}
		} else if ratio, ok := payload["aspect_ratio"].(string); ok && ratio != "" {
			current = fmt.Sprintf("Ratio: %s", ratio)
		}
	}
	return fmt.Sprintf(" | Size: %s [Press Alt+A to change]", styles.MetricValueStyle.Render(current))
}

func (m TesterModel) reasoningEffortBadges() string {
	efforts := []string{
		db.ReasoningEffortNone,
		db.ReasoningEffortLow,
		db.ReasoningEffortMedium,
		db.ReasoningEffortHigh,
		db.ReasoningEffortXHigh,
		db.ReasoningEffortMax,
	}
	var sb strings.Builder
	sb.WriteString(styles.MetricLabelStyle.Render("Reasoning Effort Shortcuts: ") + " ")
	for i, effort := range efforts {
		style := styles.BadgeStyle
		if effort == m.ReasoningEffort {
			style = styles.BadgeAccentStyle
		}
		sb.WriteString(style.Render(fmt.Sprintf("[%d] %s", i+1, effort)) + " ")
	}
	return sb.String()
}

func (m TesterModel) renderLeftPane(width, height int) string {
	color := styles.ColorMuted
	if m.ActivePane == PaneRequest {
		color = styles.ColorSecondary
	}
	var content string
	switch {
	case m.SelectingAspectRatio:
		color = styles.ColorAccent
		content = m.renderSizePicker(height)
	case m.SelectingModel:
		color = styles.ColorAccent
		content = m.renderModelPicker(height)
	default:
		title := "📝 Request Payload JSON"
		if m.ActivePane == PaneRequest {
			title += " (Active Focus)"
		}
		content = fmt.Sprintf("%s\n%s", styles.SubtitleStyle.Render(title), m.Textarea.View())
	}
	return renderTesterPane(width, height, color, content)
}

func (m TesterModel) renderSizePicker(height int) string {
	total := len(SupportedImageSizes)
	title := fmt.Sprintf("📐 Select Image Size ([%d/%d] - ↑/↓ to move, Enter to apply, Esc to cancel)", m.AspectRatioIndex+1, total)
	start, end := visibleWindow(m.AspectRatioIndex, total, max(10, height-4))
	var sb strings.Builder
	sb.WriteString(styles.SubtitleStyle.Render(title) + "\n\n")
	if start > 0 {
		sb.WriteString(styles.HelpStyle.Render(fmt.Sprintf("  ▲ ... %d smaller sizes above ...", start)) + "\n")
	}
	for i := start; i < end; i++ {
		option := SupportedImageSizes[i]
		prefix := "  "
		if i == m.AspectRatioIndex {
			prefix = "👉"
		}
		sb.WriteString(fmt.Sprintf("%s %-10s %s\n", prefix, styles.BadgeAccentStyle.Render(option.Size), styles.MetricValueStyle.Render(option.Desc)))
	}
	if end < total {
		sb.WriteString(styles.HelpStyle.Render(fmt.Sprintf("  ▼ ... %d larger sizes below ...", total-end)) + "\n")
	}
	return sb.String()
}

func (m TesterModel) renderModelPicker(height int) string {
	total := len(m.DiscoveredModels)
	title := fmt.Sprintf("🤖 Select Model (Model list length: %d, [%d/%d] - ↑/↓ to move, Enter to apply, Esc to cancel)", total, m.ModelIndex+1, total)
	start, end := visibleWindow(m.ModelIndex, total, max(20, height-4))
	var sb strings.Builder
	sb.WriteString(styles.SubtitleStyle.Render(title) + "\n\n")
	if start > 0 {
		sb.WriteString(fmt.Sprintf("  ▲ %d models above...\n", start))
	}
	for i := start; i < end; i++ {
		prefix := "  "
		if i == m.ModelIndex {
			prefix = "👉"
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", prefix, styles.MetricValueStyle.Render(m.DiscoveredModels[i])))
	}
	if end < total {
		sb.WriteString(fmt.Sprintf("  ▼ %d models below...\n", total-end))
	}
	return sb.String()
}

func (m TesterModel) renderRightPane(width, height int) string {
	title := "📊 Response JSON"
	if m.IsStreamMode {
		title = "📊 Response Stream"
	}
	color := styles.ColorMuted
	if m.ActivePane == PaneResponse {
		color = styles.ColorSecondary
		title += " (Active Scroll)"
	}

	var content strings.Builder
	content.WriteString(styles.SubtitleStyle.Render(title) + "\n")
	if m.CopyStatusMsg != "" {
		statusStyle := styles.BadgeSuccessStyle
		if m.StatusIsError {
			statusStyle = styles.ErrorStyle
		}
		content.WriteString(statusStyle.Render(m.CopyStatusMsg) + "\n")
	}
	switch {
	case m.IsExecuting && !m.IsStreamMode:
		content.WriteString("\n" + m.Spinner.View() + " Sending request payload...\n")
	case m.IsExecuting:
		content.WriteString(m.renderExecutingStream())
	case m.LastResult != nil:
		content.WriteString(m.renderCompletedResult())
	default:
		content.WriteString(lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("\nPress [Ctrl+S] to send payload and view response here."))
	}
	return renderTesterPane(width, height, color, content.String())
}

func (m TesterModel) renderExecutingStream() string {
	status := "HTTP ..."
	if m.StreamStatusCode > 0 {
		status = fmt.Sprintf("HTTP %d", m.StreamStatusCode)
	}
	metrics := fmt.Sprintf(
		"%s %s | Latency: %s",
		m.Spinner.View(),
		styles.BadgeSuccessStyle.Render(status),
		styles.MetricValueStyle.Render(formatLatency(m.StreamLatency)),
	)
	var sb strings.Builder
	sb.WriteString(metrics + "\n\n")
	if m.StreamError != "" {
		sb.WriteString(styles.ErrorStyle.Render("Error: "+errorSummary(m.StreamError)) + "\n")
	}
	sb.WriteString(m.Viewport.View())
	return sb.String()
}

func (m TesterModel) renderCompletedResult() string {
	result := m.LastResult
	statusStyle := styles.BadgeAccentStyle
	status := fmt.Sprintf("HTTP %d", result.StatusCode)
	if result.StatusCode == 200 && result.Error == "" {
		statusStyle = styles.BadgeSuccessStyle
	} else if result.Error != "" {
		status = fmt.Sprintf("HTTP %d (API Error)", result.StatusCode)
	}
	metrics := fmt.Sprintf(
		"%s | Latency: %s | tps: %s",
		statusStyle.Render(status),
		styles.MetricValueStyle.Render(formatLatency(result.Latency)),
		formatTPS(result.TotalTokens, result.Latency),
	)
	var sb strings.Builder
	sb.WriteString(metrics + "\n\n")
	if result.Error != "" {
		sb.WriteString(styles.ErrorStyle.Render("Error: "+errorSummary(result.Error)) + "\n")
	}
	sb.WriteString(m.Viewport.View())
	return sb.String()
}

func renderTesterPane(width, height int, color lipgloss.Color, content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1).
		Width(width).
		Height(height).
		Render(content)
}

func errorSummary(message string) string {
	if idx := strings.IndexAny(message, "\n\r"); idx >= 0 {
		message = message[:idx]
	}
	runes := []rune(message)
	if len(runes) > 80 {
		return string(runes[:80]) + "..."
	}
	return message
}

func (m TesterModel) renderHelp() string {
	if m.Record.APIType == api.APITypeOpenAIImages {
		return styles.HelpStyle.Render("[Ctrl+S] Generate  [Ctrl+O] Open Image  [Alt+A] Aspect Ratio  [Alt+M] Model  [Ctrl+Y] Copy Req  [Ctrl+U] Copy Resp  [PgUp/PgDn] Scroll  [Tab] Pane  [Esc] Manager")
	}
	return styles.HelpStyle.Render("[Ctrl+S] Send  [Ctrl+Y] Copy Req  [Ctrl+U] Copy Resp  [PgUp/PgDn] Scroll  [Alt+M] Model  [Alt+S] Stream  [Tab] Pane  [Alt+1~6] Reasoning  [Esc] Manager")
}
