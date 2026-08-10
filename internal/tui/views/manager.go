package views

import (
	"fmt"
	"strings"

	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ManagerModel represents the provider list and CRUD manager view
type ManagerModel struct {
	DB          *db.DB
	Records     []db.ProviderRecord
	Cursor      int
	Width       int
	Height      int
	StatusMsg   string
	IsError     bool
}

func NewManagerModel(database *db.DB) ManagerModel {
	m := ManagerModel{
		DB: database,
	}
	m.RefreshRecords()
	return m
}

func (m *ManagerModel) RefreshRecords() {
	recs, err := m.DB.ListRecords()
	if err != nil {
		m.StatusMsg = fmt.Sprintf("Failed to load records: %v", err)
		m.IsError = true
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
}

func (m ManagerModel) Update(msg tea.Msg) (ManagerModel, tea.Cmd, string) {
	var action string // e.g. "probe_new", "open_tester", "quit"

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "k", "up":
			if m.Cursor > 0 {
				m.Cursor--
			}
		case "j", "down":
			if m.Cursor < len(m.Records)-1 {
				m.Cursor++
			}
		case "n":
			action = "probe_new"
		case "enter", "t":
			if len(m.Records) > 0 && m.Cursor < len(m.Records) {
				action = "open_tester"
			}
		case "d":
			if len(m.Records) > 0 && m.Cursor < len(m.Records) {
				target := m.Records[m.Cursor]
				err := m.DB.DeleteRecord(target.ID)
				if err != nil {
					m.StatusMsg = fmt.Sprintf("Failed to delete record #%d: %v", target.ID, err)
					m.IsError = true
				} else {
					m.StatusMsg = fmt.Sprintf("Deleted record '%s'", target.Name)
					m.IsError = false
					m.RefreshRecords()
				}
			}
		case "r":
			m.RefreshRecords()
			m.StatusMsg = "Records refreshed"
			m.IsError = false
		}
	}

	return m, nil, action
}

func (m ManagerModel) SelectedRecord() *db.ProviderRecord {
	if len(m.Records) == 0 || m.Cursor >= len(m.Records) {
		return nil
	}
	return &m.Records[m.Cursor]
}

func (m ManagerModel) View() string {
	var sb strings.Builder

	header := styles.HeaderStyle.Render("⚡ LLM Provider Manager & Tester (SQLite)")
	sb.WriteString(header + "\n\n")

	if m.StatusMsg != "" {
		if m.IsError {
			sb.WriteString(styles.ErrorStyle.Render("❌ "+m.StatusMsg) + "\n\n")
		} else {
			sb.WriteString(styles.SubtitleStyle.Render("ℹ️  "+m.StatusMsg) + "\n\n")
		}
	}

	if len(m.Records) == 0 {
		emptyCard := styles.CardStyle.Render(
			lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(
				"No LLM Providers configured.\nPress 'n' to enter Base URL & API Key for auto-detection probing.",
			),
		)
		sb.WriteString(emptyCard + "\n")
	} else {
		for i, r := range m.Records {
			isCurrent := (i == m.Cursor)

			// Badge style according to api_type
			var apiTypeBadge string
			switch r.APIType {
			case db.APITypeAnthropic:
				apiTypeBadge = styles.BadgeAccentStyle.Render("Anthropic Messages")
			case db.APITypeOpenAIResponses:
				apiTypeBadge = styles.BadgeSuccessStyle.Render("OpenAI Responses")
			default:
				apiTypeBadge = styles.BadgeStyle.Render("OpenAI Chat")
			}

			reasoningText := fmt.Sprintf("Reasoning: %s", r.ReasoningEffort)

			cardContent := fmt.Sprintf(
				"%s %s\n%s | Model: %s | %s\nKey: %s",
				styles.SubtitleStyle.Render(fmt.Sprintf("#%d %s", r.ID, r.Name)),
				apiTypeBadge,
				lipgloss.NewStyle().Foreground(styles.ColorText).Render(r.BaseURL),
				styles.MetricValueStyle.Render(r.Model),
				styles.MetricLabelStyle.Render(reasoningText),
				lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(maskAPIKey(r.APIKey)),
			)

			if isCurrent {
				sb.WriteString("👉 " + styles.ActiveCardStyle.Render(cardContent) + "\n")
			} else {
				sb.WriteString("   " + styles.CardStyle.Render(cardContent) + "\n")
			}
		}
	}

	helpKey := styles.HelpStyle.Render(
		"[n] Add/Probe Provider  [Enter/t] Start Chat Test  [d] Delete Record  [r] Refresh  [q] Quit",
	)
	sb.WriteString("\n" + helpKey)

	return sb.String()
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "********"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
