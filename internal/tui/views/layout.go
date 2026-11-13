package views

import (
	"fmt"
	"strings"

	"llm_tui/internal/tui/styles"
)

func visibleWindow(cursor, total, maxVisible int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if maxVisible < 1 {
		maxVisible = 1
	}
	if maxVisible > total {
		maxVisible = total
	}
	start := cursor - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > total {
		end = total
		start = max(0, end-maxVisible)
	}
	return start, end
}

func (m TesterModel) paneDimensions() (int, int) {
	halfWidth := 40
	if m.Width > 0 {
		halfWidth = max(12, (m.Width-6)/2)
	}
	paneHeight := 16
	if m.Height > 0 {
		paneHeight = max(6, m.Height-13)
	}
	return halfWidth, paneHeight
}

func renderStatusBanner(status string, isError bool) string {
	if status == "" {
		return ""
	}
	if isError {
		return styles.ErrorStyle.Render("❌ "+status) + "\n\n"
	}
	return styles.SubtitleStyle.Render("ℹ️  "+status) + "\n\n"
}

func renderModelWindow(sb *strings.Builder, models []string, cursor, maxVisible int) {
	total := len(models)
	start, end := visibleWindow(cursor, total, maxVisible)
	if start > 0 {
		sb.WriteString(fmt.Sprintf("  ▲ %d models above...\n", start))
	}
	for i := start; i < end; i++ {
		prefix := "  "
		if i == cursor {
			prefix = "👉"
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", prefix, styles.MetricValueStyle.Render(models[i])))
	}
	if end < total {
		sb.WriteString(fmt.Sprintf("  ▼ %d models below...\n", total-end))
	}
}
