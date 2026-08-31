package views

import (
	"fmt"
	"strings"

	"llm_tui/internal/api"
	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ManagerModel represents the provider list and CRUD manager view
type ManagerModel struct {
	DB            *db.DB
	Version       string
	Records       []db.ProviderRecord
	Cursor        int
	Width         int
	Height        int
	StatusMsg     string
	IsError       bool
	ConfirmDelete bool
	Viewport      viewport.Model
	cardOffsets   [][2]int
}

func NewManagerModel(database *db.DB, version ...string) ManagerModel {
	ver := "v1.0.0"
	if len(version) > 0 && version[0] != "" {
		ver = version[0]
	}
	vp := viewport.New(76, 14)
	m := ManagerModel{
		DB:       database,
		Version:  ver,
		Viewport: vp,
		Width:    80,
		Height:   24,
	}
	m.RefreshRecords()
	return m
}

func (m *ManagerModel) recalculateDimensions() {
	cardWidth := m.Width - 6
	if cardWidth < 30 {
		cardWidth = m.Width - 2
		if cardWidth < 20 {
			cardWidth = 20
		}
	}

	headerHeight := 3
	if m.StatusMsg != "" {
		headerHeight += 2
	}
	footerHeight := 3

	availHeight := m.Height - headerHeight - footerHeight
	if availHeight < 3 {
		availHeight = 3
	}

	m.Viewport.Width = cardWidth + 2
	m.Viewport.Height = availHeight
}

func (m *ManagerModel) Resize(w, h int) {
	m.Width = w
	m.Height = h
	m.recalculateDimensions()
	m.updateViewportContent()
	m.ensureCursorVisible()
}

func (m *ManagerModel) RefreshRecords() {
	recs, err := m.DB.ListRecords()
	if err != nil {
		m.StatusMsg = fmt.Sprintf("Failed to load records: %v", err)
		m.IsError = true
		m.recalculateDimensions()
		m.updateViewportContent()
		return
	}
	m.Records = recs
	if m.Cursor >= len(m.Records) && len(m.Records) > 0 {
		m.Cursor = len(m.Records) - 1
	}
	if len(m.Records) == 0 {
		m.StatusMsg = "No provider records saved yet. Press 'n' to add a new provider!"
		m.IsError = false
	}
	m.recalculateDimensions()
	m.updateViewportContent()
	m.ensureCursorVisible()
}

func (m *ManagerModel) updateViewportContent() {
	cardWidth := m.Width - 6
	if cardWidth < 30 {
		cardWidth = m.Width - 2
		if cardWidth < 20 {
			cardWidth = 20
		}
	}

	normalCardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorMuted).
		Padding(0, 1).
		Width(cardWidth)

	activeCardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorSecondary).
		Padding(0, 1).
		Width(cardWidth)

	if len(m.Records) == 0 {
		emptyCard := normalCardStyle.Render(
			lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(
				"No LLM Providers configured.\nPress 'n' to enter Base URL & optional API Key for auto-detection probing.",
			),
		)
		m.Viewport.SetContent(emptyCard)
		m.cardOffsets = nil
		return
	}

	var sb strings.Builder
	m.cardOffsets = make([][2]int, len(m.Records))
	currentLine := 0

	for i, r := range m.Records {
		isCurrent := (i == m.Cursor)

		// Badge style according to api_type
		var apiTypeBadge string
		switch r.APIType {
		case api.APITypeOpenAIImages:
			apiTypeBadge = styles.BadgeAccentStyle.Render("🖼️ OpenAI Images")
		case api.APITypeAnthropic:
			apiTypeBadge = styles.BadgeAccentStyle.Render("Anthropic Messages")
		case api.APITypeOpenAIResponses:
			apiTypeBadge = styles.BadgeSuccessStyle.Render("OpenAI Responses")
		default:
			apiTypeBadge = styles.BadgeStyle.Render("OpenAI Chat")
		}

		pointer := "  "
		if isCurrent {
			pointer = "👉"
		}

		// Display sequential 1-indexed number (#1, #2, #3...) ordered by updated_at
		title := fmt.Sprintf("%s #%d %s", pointer, i+1, r.Name)
		line1 := fmt.Sprintf("%s  %s", styles.SubtitleStyle.Render(title), apiTypeBadge)
		line2 := fmt.Sprintf(
			"URL: %s | Model: %s | Reasoning: %s",
			lipgloss.NewStyle().Foreground(styles.ColorText).Render(r.BaseURL),
			styles.MetricValueStyle.Render(r.Model),
			styles.MetricLabelStyle.Render(r.ReasoningEffort),
		)
		line3 := fmt.Sprintf("Key: %s", lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(maskAPIKey(r.APIKey)))

		cardContent := fmt.Sprintf("%s\n%s\n%s", line1, line2, line3)
		var renderedCard string
		if isCurrent {
			renderedCard = activeCardStyle.Render(cardContent)
		} else {
			renderedCard = normalCardStyle.Render(cardContent)
		}

		cardLines := strings.Count(renderedCard, "\n") + 1
		m.cardOffsets[i] = [2]int{currentLine, currentLine + cardLines}
		sb.WriteString(renderedCard + "\n")
		currentLine += cardLines + 1
	}

	m.Viewport.SetContent(sb.String())
}

func (m *ManagerModel) ensureCursorVisible() {
	if len(m.Records) == 0 || m.Cursor < 0 || m.Cursor >= len(m.cardOffsets) {
		return
	}
	startLine := m.cardOffsets[m.Cursor][0]
	endLine := m.cardOffsets[m.Cursor][1]
	vpHeight := m.Viewport.Height
	if vpHeight <= 0 {
		return
	}

	totalLines := m.Viewport.TotalLineCount()
	if totalLines > vpHeight && m.Viewport.YOffset > totalLines-vpHeight {
		m.Viewport.SetYOffset(totalLines - vpHeight)
	} else if totalLines <= vpHeight {
		m.Viewport.SetYOffset(0)
	}

	if startLine < m.Viewport.YOffset {
		m.Viewport.SetYOffset(startLine)
	} else if endLine > m.Viewport.YOffset+vpHeight {
		m.Viewport.SetYOffset(endLine - vpHeight)
	}
}

func (m ManagerModel) Update(msg tea.Msg) (ManagerModel, tea.Cmd, string) {
	var action string // e.g. "probe_new", "open_tester", "quit"
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Cancel delete confirmation on any key except 'd'
		if m.ConfirmDelete && msg.String() != "d" {
			m.ConfirmDelete = false
			m.StatusMsg = ""
			m.IsError = false
			m.recalculateDimensions()
			m.ensureCursorVisible()
		}

		switch msg.String() {
		case "k", "up":
			if m.StatusMsg != "" && !m.ConfirmDelete {
				m.StatusMsg = ""
				m.IsError = false
				m.recalculateDimensions()
			}
			if m.Cursor > 0 {
				m.Cursor--
				m.updateViewportContent()
				m.ensureCursorVisible()
			}
		case "j", "down":
			if m.StatusMsg != "" && !m.ConfirmDelete {
				m.StatusMsg = ""
				m.IsError = false
				m.recalculateDimensions()
			}
			if m.Cursor < len(m.Records)-1 {
				m.Cursor++
				m.updateViewportContent()
				m.ensureCursorVisible()
			}
		case "pgup", "ctrl+b":
			m.Viewport.HalfViewUp()
		case "pgdown", "ctrl+f":
			m.Viewport.HalfViewDown()
		case "n":
			action = "probe_new"
		case "enter", "t":
			if len(m.Records) > 0 && m.Cursor < len(m.Records) {
				action = "open_tester"
			}
		case "d":
			if len(m.Records) > 0 && m.Cursor < len(m.Records) {
				if m.ConfirmDelete {
					// Second press: actually delete
					target := m.Records[m.Cursor]
					err := m.DB.DeleteRecord(target.ID)
					if err != nil {
						m.StatusMsg = fmt.Sprintf("Failed to delete record: %v", err)
						m.IsError = true
						m.recalculateDimensions()
					} else {
						m.StatusMsg = fmt.Sprintf("Deleted record '%s'", target.Name)
						m.IsError = false
						m.RefreshRecords()
					}
					m.ConfirmDelete = false
				} else {
					// First press: ask for confirmation
					m.ConfirmDelete = true
					m.StatusMsg = fmt.Sprintf("⚠️  Press 'd' again to confirm deleting '%s'", m.Records[m.Cursor].Name)
					m.IsError = true
					m.recalculateDimensions()
					m.ensureCursorVisible()
				}
			}
		case "q":
			action = "quit"
		}
	}

	m.Viewport, cmd = m.Viewport.Update(msg)
	return m, cmd, action
}

func (m ManagerModel) SelectedRecord() *db.ProviderRecord {
	if len(m.Records) == 0 || m.Cursor >= len(m.Records) {
		return nil
	}
	return &m.Records[m.Cursor]
}

func (m ManagerModel) View() string {
	var sb strings.Builder

	verStr := m.Version
	if verStr == "" {
		verStr = "v1.0.0"
	}
	header := styles.HeaderStyle.Render(fmt.Sprintf("⚡ LLM & Image AI Manager %s (Chat · Responses · Anthropic · Images)", verStr))
	sb.WriteString(header + "\n\n")

	if m.StatusMsg != "" {
		if m.IsError {
			sb.WriteString(styles.ErrorStyle.Render("❌ "+m.StatusMsg) + "\n\n")
		} else {
			sb.WriteString(styles.SubtitleStyle.Render("ℹ️  "+m.StatusMsg) + "\n\n")
		}
	}

	sb.WriteString(m.Viewport.View() + "\n")

	var helpParts []string
	helpParts = append(helpParts, "[n] New Provider", "[Enter/t] Test Laboratory", "[d] Delete Record", "[q] Quit App")
	if len(m.Records) > 0 {
		helpParts = append(helpParts, fmt.Sprintf("[%d/%d]", m.Cursor+1, len(m.Records)))
	}
	helpKey := styles.HelpStyle.Render(strings.Join(helpParts, "  "))
	sb.WriteString("\n" + helpKey)

	return sb.String()
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "******"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
