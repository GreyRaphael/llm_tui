package views

import (
	"fmt"
	"strings"

	"llm_tui/internal/api"
	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ManagerModel represents the provider list and CRUD manager view
type ManagerModel struct {
	DB            *db.DB
	Records       []db.ProviderRecord
	Cursor        int
	Width         int
	Height        int
	StatusMsg     string
	IsError       bool
	ConfirmDelete bool
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
		// Cancel delete confirmation on any key except 'd'
		if m.ConfirmDelete && msg.String() != "d" {
			m.ConfirmDelete = false
			m.StatusMsg = ""
			m.IsError = false
		}

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
				if m.ConfirmDelete {
					// Second press: actually delete
					target := m.Records[m.Cursor]
					err := m.DB.DeleteRecord(target.ID)
					if err != nil {
						m.StatusMsg = fmt.Sprintf("Failed to delete record: %v", err)
						m.IsError = true
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
				}
			}
		case "q":
			action = "quit"
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

	cardWidth := m.Width - 6
	if cardWidth < 60 {
		cardWidth = 72
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

			if isCurrent {
				sb.WriteString(activeCardStyle.Render(cardContent) + "\n")
			} else {
				sb.WriteString(normalCardStyle.Render(cardContent) + "\n")
			}
		}
	}

	helpKey := styles.HelpStyle.Render(
		"[n] New Provider  [Enter/t] Test Laboratory  [d] Delete Record  [q] Quit App",
	)
	sb.WriteString("\n" + helpKey)

	return sb.String()
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "******"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
